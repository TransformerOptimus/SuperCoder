package impl

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

const (
	grepMaxFiles   = 500
	grepTimeout    = 5 * time.Second
	grepContextLen = 3 // lines of context before/after match
)

type grepRetriever struct {
	provider services.SourceProvider
	root     string
}

// NewGrepRetriever creates a retriever that does regex/literal pattern matching on raw files.
func NewGrepRetriever(provider services.SourceProvider, root string) services.RetrieverService {
	return &grepRetriever{
		provider: provider,
		root:     root,
	}
}

func (r *grepRetriever) Search(ctx context.Context, query string, limit int, _ services.SearchFilter) ([]services.RetrieverResult, error) {
	ctx, cancel := context.WithTimeout(ctx, grepTimeout)
	defer cancel()

	re, err := regexp.Compile(query)
	if err != nil {
		// Fall back to literal match.
		re = regexp.MustCompile(regexp.QuoteMeta(query))
	}

	files, err := r.provider.ListFiles(ctx, r.root)
	if err != nil {
		return nil, fmt.Errorf("grep retriever list files failed: %w", err)
	}

	// Limit file count for performance.
	if len(files) > grepMaxFiles {
		files = files[:grepMaxFiles]
	}

	var results []services.RetrieverResult
	for _, f := range files {
		if ctx.Err() != nil {
			break
		}
		if f.IsDir || isBinaryExtension(filepath.Ext(f.RelativePath)) {
			continue
		}

		data, err := r.provider.ReadFile(ctx, f.Path)
		if err != nil {
			continue
		}

		content := string(data)
		lines := strings.Split(content, "\n")

		locs := re.FindAllStringIndex(content, -1)
		if len(locs) == 0 {
			continue
		}

		// Find line numbers for matches and extract context.
		seen := make(map[int]bool)
		for _, loc := range locs {
			lineNum := strings.Count(content[:loc[0]], "\n")
			if seen[lineNum] {
				continue
			}
			seen[lineNum] = true

			start := lineNum - grepContextLen
			if start < 0 {
				start = 0
			}
			end := lineNum + grepContextLen + 1
			if end > len(lines) {
				end = len(lines)
			}

			snippet := strings.Join(lines[start:end], "\n")
			results = append(results, services.RetrieverResult{
				ChunkID:  fmt.Sprintf("%s:%d", f.RelativePath, lineNum+1),
				Content:  snippet,
				FilePath: f.RelativePath,
				Score:    1.0,
				Source:   "grep",
			})

			if len(results) >= limit {
				return results, nil
			}
		}
	}

	return results, nil
}

func (r *grepRetriever) FindFunction(ctx context.Context, functionName string, filter services.SearchFilter) ([]services.RetrieverResult, error) {
	// Build a regex that matches function definitions across common languages.
	pattern := fmt.Sprintf(`(?:func|def|function|fn)\s+(?:\([^)]*\)\s*)?%s\b`, regexp.QuoteMeta(functionName))
	return r.Search(ctx, pattern, 5, filter)
}

func (r *grepRetriever) Name() string { return "grep" }

func isBinaryExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".svg",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z",
		".exe", ".dll", ".so", ".dylib", ".bin",
		".pdf", ".doc", ".docx", ".xls", ".xlsx",
		".wasm", ".class", ".o", ".a", ".pyc":
		return true
	}
	return false
}

func (r *grepRetriever) ListFiles(_ context.Context, _ services.SearchFilter) ([]string, error) {
	return nil, nil
}

func (r *grepRetriever) ReadFile(_ context.Context, _ string, _ services.SearchFilter) (string, error) {
	return "", nil
}
