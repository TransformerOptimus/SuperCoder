package impl

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// lineContentCap is the maximum number of characters the model is asked to
// echo back in <line_content>. Longer lines are safe: we truncate both sides
// to this length before fuzzy comparison.
const lineContentCap = 120

// anchorSearchWindow is the number of lines on either side of a claimed line
// that are searched for a content match when re-anchoring.
const anchorSearchWindow int32 = 10

// anchorAcceptThreshold is the minimum similarity ratio required to accept a
// line_content match (at the claimed line OR at a re-anchor candidate).
const anchorAcceptThreshold = 0.8

// DiffIndex summarizes a unified diff as per-file maps from new-side line
// numbers to line content. Only commentable lines (added `+` and context ` `
// on the new side) are indexed; deleted lines are excluded.
type DiffIndex struct {
	byFile map[string]map[int32]string
}

func newDiffIndex() *DiffIndex {
	return &DiffIndex{byFile: make(map[string]map[int32]string)}
}

// IsValidLine reports whether line is a commentable new-side line for file.
func (d *DiffIndex) IsValidLine(file string, line int32) bool {
	lines, ok := d.byFile[file]
	if !ok {
		return false
	}
	_, ok = lines[line]
	return ok
}

// Content returns the content of the commentable line, stripped of the
// unified-diff prefix. Returns "" if the line is not indexed.
func (d *DiffIndex) Content(file string, line int32) string {
	if lines, ok := d.byFile[file]; ok {
		return lines[line]
	}
	return ""
}

// Nearby returns all commentable (line, content) pairs for file within
// [line-window, line+window].
func (d *DiffIndex) Nearby(file string, line, window int32) map[int32]string {
	result := make(map[int32]string)
	lines, ok := d.byFile[file]
	if !ok {
		return result
	}
	for l, c := range lines {
		if l >= line-window && l <= line+window {
			result[l] = c
		}
	}
	return result
}

var (
	diffFileRe = regexp.MustCompile(`^diff --git a/.+ b/(.+)$`)
	diffHunkRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
)

// parseDiffIndex walks a unified diff and records each new-side commentable
// line's content.
func parseDiffIndex(diff string) *DiffIndex {
	idx := newDiffIndex()

	var currentFile string
	var newLine int32
	inHunk := false

	for _, line := range strings.Split(diff, "\n") {
		if fm := diffFileRe.FindStringSubmatch(line); fm != nil {
			currentFile = fm[1]
			inHunk = false
			if _, ok := idx.byFile[currentFile]; !ok {
				idx.byFile[currentFile] = make(map[int32]string)
			}
			continue
		}
		if currentFile == "" {
			continue
		}
		if hm := diffHunkRe.FindStringSubmatch(line); hm != nil {
			start, _ := strconv.Atoi(hm[1])
			newLine = int32(start)
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(line, "-"):
			// Deleted line — not commentable, does not advance new-side counter.
		case strings.HasPrefix(line, "+"):
			idx.byFile[currentFile][newLine] = line[1:]
			newLine++
		case strings.HasPrefix(line, " "):
			idx.byFile[currentFile][newLine] = line[1:]
			newLine++
		default:
			// "\ No newline at end of file", blank trailing split artifact, or a
			// malformed line — do not index, do not advance.
		}
	}
	return idx
}

// numberedDiffForFiles renders per-file patches with [NNNN] line-number
// prefixes on every commentable new-side line. Deleted lines are shown with
// a blank prefix `[    ]` so the model can see them for context but cannot
// cite a new-side number for them.
func numberedDiffForFiles(files []services.PRFileInfo) string {
	var sb strings.Builder

	for _, f := range files {
		if f.Patch == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("File: %s (status: %s, +%d -%d)\n",
			f.Filename, f.Status, f.Additions, f.Deletions))

		var newLine int32
		inHunk := false

		for _, line := range strings.Split(f.Patch, "\n") {
			if hm := diffHunkRe.FindStringSubmatch(line); hm != nil {
				start, _ := strconv.Atoi(hm[1])
				newLine = int32(start)
				inHunk = true
				sb.WriteString(line)
				sb.WriteByte('\n')
				continue
			}
			if !inHunk {
				sb.WriteString(line)
				sb.WriteByte('\n')
				continue
			}
			switch {
			case strings.HasPrefix(line, "-"):
				sb.WriteString(fmt.Sprintf("[    ]%s\n", line))
			case strings.HasPrefix(line, "+"):
				sb.WriteString(fmt.Sprintf("[%4d]%s\n", newLine, line))
				newLine++
			case strings.HasPrefix(line, " "):
				sb.WriteString(fmt.Sprintf("[%4d]%s\n", newLine, line))
				newLine++
			default:
				// "\ No newline at end of file", blank trailing split artifact,
				// or a malformed line — emit verbatim without a number and do
				// not advance the counter.
				sb.WriteString(line)
				sb.WriteByte('\n')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// similarityRatio returns a value in [0,1] representing how similar two code
// lines are after whitespace normalization. 1.0 is an exact match; 0.0 is
// completely different. Uses normalized Levenshtein distance.
func similarityRatio(a, b string) float64 {
	a = normalizeForCompare(a)
	b = normalizeForCompare(b)
	if a == "" && b == "" {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.0
	}
	if len(a) > lineContentCap {
		a = a[:lineContentCap]
	}
	if len(b) > lineContentCap {
		b = b[:lineContentCap]
	}
	d := levenshtein(a, b)
	m := len(a)
	if len(b) > m {
		m = len(b)
	}
	return 1.0 - float64(d)/float64(m)
}

// normalizeForCompare trims outer whitespace and collapses internal runs of
// spaces/tabs to a single space, so indentation changes don't swamp the score.
func normalizeForCompare(s string) string {
	s = strings.TrimSpace(s)
	var sb strings.Builder
	sb.Grow(len(s))
	inSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' {
			if !inSpace {
				sb.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		inSpace = false
		sb.WriteByte(c)
	}
	return sb.String()
}

// levenshtein computes the edit distance between a and b using a rolling
// two-row DP. O(len(a)*len(b)) time, O(min(len)) space.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
