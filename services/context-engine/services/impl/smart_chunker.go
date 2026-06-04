package impl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type smartChunker struct {
	maxTokens      int
	tokensPerChar  float64
	minMergeTokens int
}

func NewSmartChunker(cfg config.IndexerConfig) *smartChunker {
	minMerge := cfg.ChunkMinMergeTokens()
	if minMerge <= 0 {
		minMerge = 64
	}
	return &smartChunker{
		maxTokens:      cfg.ChunkMaxTokens(),
		tokensPerChar:  cfg.TokensPerChar(),
		minMergeTokens: minMerge,
	}
}

func (c *smartChunker) ChunkElements(elements []services.CodeElement) []services.CodeChunk {
	// Group elements by file.
	byFile := make(map[string][]services.CodeElement)
	for _, el := range elements {
		byFile[el.FilePath] = append(byFile[el.FilePath], el)
	}

	var allChunks []services.CodeChunk

	for filePath, fileElements := range byFile {
		// Sort by start line.
		sort.Slice(fileElements, func(i, j int) bool {
			return fileElements[i].StartLine < fileElements[j].StartLine
		})

		chunks := c.chunkFile(filePath, fileElements)
		allChunks = append(allChunks, chunks...)
	}

	return allChunks
}

func (c *smartChunker) chunkFile(filePath string, elements []services.CodeElement) []services.CodeChunk {
	var chunks []services.CodeChunk

	var pending []services.CodeElement
	var pendingTokens int

	flush := func() {
		if len(pending) == 0 {
			return
		}
		chunk := c.mergeToChunk(pending)
		chunks = append(chunks, chunk)
		pending = nil
		pendingTokens = 0
	}

	for _, el := range elements {
		tokens := c.estimateTokens(el.Body)

		if tokens > c.maxTokens {
			// Flush any pending small elements first.
			flush()
			// Split oversized element.
			splitChunks := c.splitElement(el)
			chunks = append(chunks, splitChunks...)
			continue
		}

		if tokens < c.minMergeTokens {
			// Small element — try to merge with siblings.
			if pendingTokens+tokens > c.maxTokens/2 {
				flush()
			}
			pending = append(pending, el)
			pendingTokens += tokens
			continue
		}

		// Normal-sized element — flush pending, emit as own chunk.
		flush()
		chunks = append(chunks, c.elementToChunk(el))
	}

	flush()
	return chunks
}

func (c *smartChunker) estimateTokens(body string) int {
	return int(float64(len(body)) * c.tokensPerChar)
}

func (c *smartChunker) mergeToChunk(elements []services.CodeElement) services.CodeChunk {
	if len(elements) == 1 {
		return c.elementToChunk(elements[0])
	}

	first := elements[0]
	last := elements[len(elements)-1]

	var bodyParts []string
	var names []string
	for _, el := range elements {
		bodyParts = append(bodyParts, el.Body)
		if el.Name != "" && el.Name != el.FilePath {
			names = append(names, el.Name)
		}
	}

	body := strings.Join(bodyParts, "\n\n")
	name := strings.Join(names, ", ")
	if name == "" {
		name = first.FilePath
	}

	header := fmt.Sprintf("// file: %s\n// %s: %s\n", first.FilePath, first.Kind, name)
	content := header + body

	return makeChunk(first.FilePath, first.StartLine, last.EndLine, content, first.Language, first.Kind, name, first.GithubOrgID, first.WorkspaceID, first.UserID, first.MachineID, first.RepoID)
}

func (c *smartChunker) elementToChunk(el services.CodeElement) services.CodeChunk {
	header := fmt.Sprintf("// file: %s\n// %s: %s\n", el.FilePath, el.Kind, el.Name)
	content := header + el.Body

	return makeChunk(el.FilePath, el.StartLine, el.EndLine, content, el.Language, el.Kind, el.Name, el.GithubOrgID, el.WorkspaceID, el.UserID, el.MachineID, el.RepoID)
}

func (c *smartChunker) splitElement(el services.CodeElement) []services.CodeChunk {
	lines := strings.Split(el.Body, "\n")
	maxLinesPerChunk := c.maxTokens // Rough estimate: 1 token ≈ 1 line for split sizing.
	if c.tokensPerChar > 0 {
		// Estimate average chars per line, then lines per chunk.
		avgCharsPerLine := float64(len(el.Body)) / float64(len(lines))
		tokensPerLine := avgCharsPerLine * c.tokensPerChar
		if tokensPerLine > 0 {
			maxLinesPerChunk = int(float64(c.maxTokens) / tokensPerLine)
		}
	}
	if maxLinesPerChunk < 10 {
		maxLinesPerChunk = 10
	}

	var chunks []services.CodeChunk
	partNum := 0

	for start := 0; start < len(lines); {
		end := start + maxLinesPerChunk
		if end > len(lines) {
			end = len(lines)
		}

		// Try to split at a blank line or closing brace near the boundary.
		if end < len(lines) {
			bestSplit := end
			for i := end; i > start+maxLinesPerChunk/2; i-- {
				trimmed := strings.TrimSpace(lines[i-1])
				if trimmed == "" || trimmed == "}" || trimmed == "end" {
					bestSplit = i
					break
				}
			}
			end = bestSplit
		}

		partNum++
		body := strings.Join(lines[start:end], "\n")
		startLine := el.StartLine + uint32(start)
		endLine := el.StartLine + uint32(end-1)

		header := fmt.Sprintf("// file: %s\n// %s: %s (part %d)\n", el.FilePath, el.Kind, el.Name, partNum)
		content := header + body

		chunks = append(chunks, makeChunk(el.FilePath, startLine, endLine, content, el.Language, el.Kind, el.Name, el.GithubOrgID, el.WorkspaceID, el.UserID, el.MachineID, el.RepoID))

		start = end
	}

	return chunks
}

func makeChunk(filePath string, startLine, endLine uint32, content, language, kind, name, githubOrgID, workspaceID, userID, machineID string, repoID uint) services.CodeChunk {
	id := fmt.Sprintf("%s:%d-%d", filePath, startLine, endLine)
	hash := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(hash[:])

	return services.CodeChunk{
		ID:      id,
		Content: content,
		Metadata: map[string]interface{}{
			"file_path":     filePath,
			"language":      language,
			"kind":          kind,
			"name":          name,
			"start_line":    startLine,
			"end_line":      endLine,
			"content_hash":  contentHash,
			"github_org_id": githubOrgID,
			"workspace_id":  workspaceID,
			"user_id":       userID,
			"machine_id":    machineID,
			"repo_id":       repoID,
		},
	}
}
