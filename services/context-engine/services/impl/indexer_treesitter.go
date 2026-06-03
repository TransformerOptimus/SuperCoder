package impl

import (
	"context"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type treeSitterIndexer struct {
	cfg    config.IndexerConfig
	logger *zap.Logger
}

func NewTreeSitterIndexer(cfg config.IndexerConfig, logger *zap.Logger) services.IndexerService {
	return &treeSitterIndexer{
		cfg:    cfg,
		logger: logger.Named("treesitter-indexer"),
	}
}

func (t *treeSitterIndexer) IndexDirectory(ctx context.Context, provider services.SourceProvider, root string) (*services.IndexResult, error) {
	files, err := provider.ListFiles(ctx, root)
	if err != nil {
		return nil, err
	}

	return t.indexFiles(ctx, provider, files)
}

func (t *treeSitterIndexer) IndexFiles(ctx context.Context, provider services.SourceProvider, root string, changedFiles []string) (*services.IndexResult, error) {
	if len(changedFiles) == 0 {
		return t.IndexDirectory(ctx, provider, root)
	}

	files := make([]services.FileInfo, 0, len(changedFiles))
	for _, f := range changedFiles {
		absPath := f
		if root != "" && !filepath.IsAbs(f) {
			absPath = filepath.Join(root, f)
		}
		files = append(files, services.FileInfo{
			Path:         absPath,
			RelativePath: f,
		})
	}

	return t.indexFiles(ctx, provider, files)
}

func (t *treeSitterIndexer) SupportedLanguages() []string {
	return t.cfg.SupportedLanguages()
}

func (t *treeSitterIndexer) indexFiles(ctx context.Context, provider services.SourceProvider, files []services.FileInfo) (*services.IndexResult, error) {
	skipDirs := make(map[string]bool)
	for k, v := range DefaultSkipDirs {
		skipDirs[k] = v
	}
	for _, d := range t.cfg.SkipDirs() {
		skipDirs[d] = true
	}

	// Filter to files with known extensions.
	var supported []services.FileInfo
	for _, f := range files {
		relPath := f.RelativePath
		if relPath == "" {
			relPath = f.Path
		}

		// Skip files in excluded directories.
		if shouldSkipPath(relPath, skipDirs) {
			continue
		}

		ext := filepath.Ext(relPath)
		base := filepath.Base(relPath)

		// Handle extensionless files like BUILD, Makefile, etc.
		lang := languageFromExtension(ext)
		if lang == "" {
			// Check base name for BUILD files.
			if base == "BUILD" || base == "BUILD.bazel" {
				lang = LangStarlark
			} else {
				continue
			}
		}

		supported = append(supported, f)
	}

	if len(supported) == 0 {
		return &services.IndexResult{}, nil
	}

	concurrency := 8
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	var mu sync.Mutex
	var allElements []services.CodeElement
	var allRelationships []services.CodeRelationship

	for _, fi := range supported {
		fi := fi
		g.Go(func() error {
			relPath := fi.RelativePath
			if relPath == "" {
				relPath = fi.Path
			}

			content, err := provider.ReadFile(gctx, fi.Path)
			if err != nil {
				t.logger.Warn("Failed to read file", zap.String("path", relPath), zap.Error(err))
				return nil // Don't fail the entire batch.
			}

			if len(content) == 0 {
				return nil
			}

			ext := filepath.Ext(relPath)
			base := filepath.Base(relPath)
			lang := languageFromExtension(ext)
			if lang == "" && (base == "BUILD" || base == "BUILD.bazel") {
				lang = LangStarlark
			}

			elements, relationships, err := extractElements(gctx, content, lang, relPath)
			if err != nil {
				t.logger.Warn("Failed to parse file", zap.String("path", relPath), zap.String("lang", lang), zap.Error(err))
				return nil
			}

			mu.Lock()
			allElements = append(allElements, elements...)
			allRelationships = append(allRelationships, relationships...)
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	t.logger.Info("Tree-sitter indexing complete",
		zap.Int("elements", len(allElements)),
		zap.Int("relationships", len(allRelationships)),
		zap.Int("files_processed", len(supported)))

	return &services.IndexResult{
		Elements:      allElements,
		Relationships: allRelationships,
	}, nil
}

func shouldSkipPath(relPath string, skipDirs map[string]bool) bool {
	parts := filepath.SplitList(relPath)
	if len(parts) <= 1 {
		// filepath.SplitList uses path list separator, not directory separator.
		// Split manually by "/".
		parts = splitPath(relPath)
	}
	for _, p := range parts {
		if skipDirs[p] {
			return true
		}
	}
	return false
}

func splitPath(path string) []string {
	dir := filepath.Dir(path)
	if dir == "." || dir == "/" {
		return []string{filepath.Base(path)}
	}
	return append(splitPath(dir), filepath.Base(path))
}
