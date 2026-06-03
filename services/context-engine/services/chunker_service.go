package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// CodeChunk represents a chunk of code ready for embedding.
type CodeChunk struct {
	ID       string
	Content  string
	Metadata map[string]interface{}
}

type ChunkerService interface {
	ChunkElements(elements []CodeElement) []CodeChunk
}

type chunkerServiceImpl struct {
	maxTokens int
}

func NewChunkerService(maxTokens int) ChunkerService {
	return &chunkerServiceImpl{maxTokens: maxTokens}
}

func (c *chunkerServiceImpl) ChunkElements(elements []CodeElement) []CodeChunk {
	var chunks []CodeChunk
	for _, el := range elements {
		id := fmt.Sprintf("%s:%d-%d", el.FilePath, el.StartLine, el.EndLine)

		header := fmt.Sprintf("// file: %s\n// %s: %s\n", el.FilePath, el.Kind, el.Name)
		content := header + el.Body

		hash := sha256.Sum256([]byte(content))
		contentHash := hex.EncodeToString(hash[:])

		chunks = append(chunks, CodeChunk{
			ID:      id,
			Content: content,
			Metadata: map[string]interface{}{
				"file_path":     el.FilePath,
				"language":      el.Language,
				"kind":          el.Kind,
				"name":          el.Name,
				"start_line":    el.StartLine,
				"end_line":      el.EndLine,
				"content_hash":  contentHash,
				"github_org_id": el.GithubOrgID,
				"workspace_id":  el.WorkspaceID,
				"user_id":       el.UserID,
				"machine_id":    el.MachineID,
				"repo_id":       el.RepoID,
			},
		})
	}
	return chunks
}
