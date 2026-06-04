package impl

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// parseDiffLineLimits parses a unified diff and returns the maximum new-side
// line number touched in each file.
func parseDiffLineLimits(diff string) map[string]int32 {
	limits := make(map[string]int32)
	if diff == "" {
		return limits
	}

	hunkRe := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
	fileRe := regexp.MustCompile(`^diff --git a/.+ b/(.+)$`)

	var currentFile string
	for _, line := range strings.Split(diff, "\n") {
		if fm := fileRe.FindStringSubmatch(line); fm != nil {
			currentFile = fm[1]
			continue
		}
		if currentFile == "" {
			continue
		}
		if hm := hunkRe.FindStringSubmatch(line); hm != nil {
			start, _ := strconv.Atoi(hm[1])
			count := 0
			if hm[2] != "" {
				count, _ = strconv.Atoi(hm[2])
			} else {
				count = 1
			}
			endLine := int32(start + count - 1)
			if count == 0 {
				endLine = int32(start)
			}
			if endLine > limits[currentFile] {
				limits[currentFile] = endLine
			}
		}
	}
	return limits
}

// anchorOutcome describes what happened when we tried to anchor an issue.
// Used for structured logging so operators can spot anchoring drift.
type anchorOutcome string

const (
	anchorExact        anchorOutcome = "exact"         // claimed line matched content >= threshold
	anchorReAnchored   anchorOutcome = "re_anchored"   // snapped to nearby line with matching content
	anchorNoContentOK  anchorOutcome = "no_content_ok" // model omitted line_content; line valid → accepted
	anchorInvalidLine  anchorOutcome = "invalid_line"  // claimed line not in diff → dropped
	anchorNoMatch      anchorOutcome = "no_match"      // content didn't match anywhere nearby → dropped
	anchorMissingFile  anchorOutcome = "missing_file"  // file not in diff → dropped
	anchorMissingField anchorOutcome = "missing_field" // file/line empty → dropped
)

// resolveAnchor validates and, if needed, re-anchors a single issue against
// the diff index. Returns the final line and the outcome for logging.
// finalLine == 0 means the issue should be dropped.
func resolveAnchor(issue services.ReviewIssue, idx *DiffIndex) (int32, anchorOutcome, float64) {
	if issue.File == "" || issue.Line <= 0 {
		return 0, anchorMissingField, 0
	}
	if _, ok := idx.byFile[issue.File]; !ok {
		return 0, anchorMissingFile, 0
	}
	claimed := int32(issue.Line)

	// No content anchor: fall back to line-validity check only.
	if strings.TrimSpace(issue.LineContent) == "" {
		if idx.IsValidLine(issue.File, claimed) {
			return claimed, anchorNoContentOK, 0
		}
		return 0, anchorInvalidLine, 0
	}

	// Check the claimed line first.
	if idx.IsValidLine(issue.File, claimed) {
		score := similarityRatio(issue.LineContent, idx.Content(issue.File, claimed))
		if score >= anchorAcceptThreshold {
			return claimed, anchorExact, score
		}
	}

	// Scan the window for the best match.
	nearby := idx.Nearby(issue.File, claimed, anchorSearchWindow)
	var bestLine int32
	var bestScore float64
	for ln, content := range nearby {
		score := similarityRatio(issue.LineContent, content)
		if score > bestScore {
			bestScore = score
			bestLine = ln
		}
	}
	if bestScore >= anchorAcceptThreshold && bestLine > 0 {
		return bestLine, anchorReAnchored, bestScore
	}

	if !idx.IsValidLine(issue.File, claimed) {
		return 0, anchorInvalidLine, bestScore
	}
	return 0, anchorNoMatch, bestScore
}

// toProtoComments converts review issues to PR review comment inputs. Each
// issue is anchored to the diff via a two-step process:
//
//  1. The claimed line must be a commentable new-side line.
//  2. Its <line_content> must match the actual diff content with similarity
//     >= anchorAcceptThreshold, or a nearby line must match within
//     anchorSearchWindow.
//
// Issues failing both steps are dropped — preferring silence over a
// misplaced comment on unrelated code. Each decision is logged.
func toProtoComments(issues []services.ReviewIssue, diff string, logger *zap.Logger) []services.PRReviewCommentInput {
	idx := parseDiffIndex(diff)

	var comments []services.PRReviewCommentInput
	var kept, dropped, reAnchored int

	for _, issue := range issues {
		line, outcome, score := resolveAnchor(issue, idx)

		fields := []zap.Field{
			zap.String("file", issue.File),
			zap.Int("claimed_line", issue.Line),
			zap.Int32("final_line", line),
			zap.String("outcome", string(outcome)),
			zap.Float64("score", score),
			zap.String("pattern", issue.Pattern),
		}

		switch outcome {
		case anchorExact, anchorNoContentOK:
			kept++
			logger.Debug("inline comment anchored", fields...)
		case anchorReAnchored:
			kept++
			reAnchored++
			logger.Info("inline comment re-anchored to nearby line", fields...)
		default:
			dropped++
			preview := issue.LineContent
			if len(preview) > 80 {
				preview = preview[:80] + "…"
			}
			logger.Warn("inline comment dropped — anchor failed",
				append(fields, zap.String("line_content_preview", preview))...)
			continue
		}

		comments = append(comments, services.PRReviewCommentInput{
			Path: issue.File,
			Line: line,
			Body: formatInlineComment(issue),
			Side: "RIGHT",
		})
	}

	if kept+dropped > 0 {
		logger.Info("anchor summary",
			zap.Int("kept", kept),
			zap.Int("dropped", dropped),
			zap.Int("re_anchored", reAnchored),
			zap.Int("total_issues", kept+dropped))
	}
	return comments
}


// toSaveComments converts review issues to the format used for persisting reviews.
func toSaveComments(issues []services.ReviewIssue) []services.SaveReviewComment {
	if len(issues) == 0 {
		return nil
	}
	comments := make([]services.SaveReviewComment, 0, len(issues))
	for _, issue := range issues {
		comments = append(comments, services.SaveReviewComment{
			FilePath:   issue.File,
			LineNumber: int32(issue.Line),
			Severity:   issue.Severity,
			Category:   issue.Category,
			Body:       formatCommentBody(issue),
		})
	}
	return comments
}

// formatInlineComment renders a finding for both GitHub posts and DB persistence.
func formatInlineComment(issue services.ReviewIssue) string {
	return formatCommentBody(issue)
}

// formatCommentBody emits `severity | category`, bold title, WHAT/WHY narrative, and fix block.
func formatCommentBody(issue services.ReviewIssue) string {
	var sb strings.Builder

	severity := strings.ToLower(strings.TrimSpace(issue.Severity))
	if severity == "" {
		severity = "note"
	}
	category := strings.TrimSpace(issue.Category)

	if category != "" {
		sb.WriteString(fmt.Sprintf("`%s` | `%s`\n\n", severity, category))
	} else {
		sb.WriteString(fmt.Sprintf("`%s`\n\n", severity))
	}

	title := strings.TrimSpace(issue.Description)
	if title == "" {
		title = "Review finding"
	}
	sb.WriteString("**" + title + "**\n\n")

	if problem := strings.TrimSpace(issue.Problem); problem != "" {
		sb.WriteString(problem)
		sb.WriteString("\n\n")
	}

	if fix := renderFix(issue.Fix); fix != "" {
		sb.WriteString(fix)
		sb.WriteString("\n\n")
	}

	if issue.Pattern != "" {
		sb.WriteString(fmt.Sprintf("<sub>Pattern: `%s`</sub>", issue.Pattern))
	}

	return strings.TrimRight(sb.String(), "\n")
}

// renderFix turns <fix> into a suggestion block, a fenced code block, or prose.
func renderFix(fix string) string {
	fix = strings.TrimSpace(fix)
	if fix == "" {
		return ""
	}

	if strings.HasPrefix(fix, "```suggestion") || strings.HasPrefix(fix, "```\nsuggestion") {
		return fix
	}

	if strings.HasPrefix(fix, "```") {
		body, ok := parseFencedBody(fix)
		if ok && strings.Count(body, "\n") <= 7 {
			return "```suggestion\n" + body + "\n```"
		}
		return fix
	}

	if !strings.Contains(fix, "\n") && looksLikeCodeLine(fix) {
		return "```suggestion\n" + fix + "\n```"
	}

	return "**Suggested fix:** " + fix
}

// parseFencedBody extracts the inner body of a ```...``` block, preserving indentation.
func parseFencedBody(s string) (string, bool) {
	if !strings.HasPrefix(s, "```") {
		return "", false
	}
	rest := s[3:]
	nl := strings.Index(rest, "\n")
	if nl < 0 {
		return "", false
	}
	body := strings.TrimRight(rest[nl+1:], "\n")
	if end := strings.LastIndex(body, "```"); end >= 0 {
		body = strings.TrimRight(body[:end], "\n")
	}
	return body, true
}

// looksLikeCodeLine flags single-line strings dense in code-ish characters.
func looksLikeCodeLine(s string) bool {
	for _, c := range s {
		switch c {
		case '=', ';', '{', '}', '(', ')', '<', '>', '[', ']':
			return true
		}
	}
	return false
}

// mapVerdictToEvent maps a review verdict to a GitHub PR review event type.
func mapVerdictToEvent() string {
	return "COMMENT"
}

// reconstructDiff builds a unified diff string from per-file patches.
// This is used for line validation when posting inline comments.
func reconstructDiff(files []services.PRFileInfo) string {
	var sb strings.Builder
	for _, f := range files {
		if f.Patch == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", f.Filename, f.Filename))
		sb.WriteString(fmt.Sprintf("--- a/%s\n", f.Filename))
		sb.WriteString(fmt.Sprintf("+++ b/%s\n", f.Filename))
		sb.WriteString(f.Patch)
		if !strings.HasSuffix(f.Patch, "\n") {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// formatCalleeBodies emits the bodies of first-hop callees so the model can
// trust API contracts without fishing. Keys are "file:fn"; sorted for cache stability.
func formatCalleeBodies(bodies map[string]string) string {
	if len(bodies) == 0 {
		return ""
	}
	keys := make([]string, 0, len(bodies))
	for k := range bodies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString("\n\n<callee_bodies>\n")
	sb.WriteString("Source of functions called by changed code. Use to verify API contracts and trace data flow without speculation.\n\n")
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("<callee key=\"%s\">\n%s\n</callee>\n\n", k, bodies[k]))
	}
	sb.WriteString("</callee_bodies>")
	return sb.String()
}

// formatFileContents emits full source for changed files in sorted order.
// Sorted iteration is required for stable prefix bytes (Anthropic prompt caching).
func formatFileContents(contents map[string]string) string {
	if len(contents) == 0 {
		return ""
	}
	paths := make([]string, 0, len(contents))
	for path := range contents {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var sb strings.Builder
	sb.WriteString("\n\n<full_file_contents>\n")
	sb.WriteString("Below are the complete source files for the changed files. Use these to understand data flow, ")
	sb.WriteString("trace how user input reaches dangerous functions, and verify API usage.\n\n")
	for _, path := range paths {
		sb.WriteString(fmt.Sprintf("<file path=\"%s\">\n%s\n</file>\n\n", path, contents[path]))
	}
	sb.WriteString("</full_file_contents>")
	return sb.String()
}

// buildFileDiffMessage formats per-file patches for the LLM with metadata
// headers and `[NNNN]` line-number prefixes on every commentable new-side
// line so the model can cite exact, pre-validated line numbers.
func buildFileDiffMessage(files []services.PRFileInfo) string {
	return numberedDiffForFiles(files)
}

// rawOutputPreviewChars caps how much of the raw model response we log when
// schema mismatch is suspected (long output, zero parsed issues).
const rawOutputPreviewChars = 1500

// logRawIfSchemaMismatch dumps a head+tail preview when the model returned a
// substantial response that yielded no parsed issues — a strong signal that
// findings landed in prose / wrong tags.
func logRawIfSchemaMismatch(logger *zap.Logger, raw string, parsed *services.ReviewOutput) {
	if logger == nil || parsed == nil {
		return
	}
	if len(parsed.Issues) > 0 {
		return
	}
	if len(raw) < 1000 {
		return
	}
	head := raw
	if len(head) > rawOutputPreviewChars {
		head = head[:rawOutputPreviewChars]
	}
	tail := ""
	if len(raw) > rawOutputPreviewChars*2 {
		tail = raw[len(raw)-rawOutputPreviewChars:]
	}
	logger.Warn("schema mismatch suspected — long model response with zero parsed issues",
		zap.Int("raw_chars", len(raw)),
		zap.String("raw_head", head),
		zap.String("raw_tail", tail))
}

// parseRawToReviewOutput parses raw LLM output text into a structured ReviewOutput.
func parseRawToReviewOutput(raw string) *services.ReviewOutput {
	output := &services.ReviewOutput{}

	reviewBlockRe := regexp.MustCompile(`(?s)<review>(.*)</review>`)
	issueBlockRe := regexp.MustCompile(`(?s)<issue>(.*?)</issue>`)
	verdictXMLRe := regexp.MustCompile(`(?s)<verdict>\s*(APPROVE|REQUEST_CHANGES|NEEDS_DISCUSSION)\s*</verdict>`)
	verdictMDRe := regexp.MustCompile(`(?i)VERDICT:\s*(APPROVE|REQUEST_CHANGES|NEEDS_DISCUSSION)`)

	extractTag := func(block, tag string) string {
		re := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(tag) + `>(.*?)</` + regexp.QuoteMeta(tag) + `>`)
		if m := re.FindStringSubmatch(block); m != nil {
			return strings.TrimSpace(m[1])
		}
		return ""
	}

	if reviewMatch := reviewBlockRe.FindStringSubmatch(raw); reviewMatch != nil {
		body := reviewMatch[1]

		output.Overview = extractTag(body, "overview")
		output.RiskLevel = extractTag(body, "risk_level")
		output.RiskReason = extractTag(body, "risk_reason")
		output.AreasAffected = extractTag(body, "areas_affected")

		if vm := verdictXMLRe.FindStringSubmatch(body); vm != nil {
			output.Verdict = strings.ToUpper(strings.TrimSpace(vm[1]))
		}

		// Parse resolved existing comments
		if resolvedRaw := extractTag(body, "resolved_comments"); resolvedRaw != "" {
			for _, id := range strings.Split(resolvedRaw, ",") {
				id = strings.TrimSpace(id)
				if id != "" {
					output.ResolvedCommentIDs = append(output.ResolvedCommentIDs, id)
				}
			}
		}

		for _, m := range issueBlockRe.FindAllStringSubmatch(body, -1) {
			issueBody := m[1]
			line, _ := strconv.Atoi(extractTag(issueBody, "line"))
			usesContext := strings.EqualFold(strings.TrimSpace(extractTag(issueBody, "uses_context")), "true")
			output.Issues = append(output.Issues, services.ReviewIssue{
				File:        extractTag(issueBody, "file"),
				Line:        line,
				LineContent: extractTag(issueBody, "line_content"),
				Severity:    strings.ToLower(extractTag(issueBody, "severity")),
				Category:    extractTag(issueBody, "category"),
				Pattern:     extractTag(issueBody, "pattern"),
				Description: extractTag(issueBody, "description"),
				Problem:     extractTag(issueBody, "problem"),
				Fix:         extractTag(issueBody, "fix"),
				UsesContext: usesContext,
			})
		}
	} else {
		if vm := verdictMDRe.FindStringSubmatch(raw); vm != nil {
			output.Verdict = strings.ToUpper(strings.TrimSpace(vm[1]))
		}
	}

	return output
}
