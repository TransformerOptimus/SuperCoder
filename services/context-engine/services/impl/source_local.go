package impl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type localSourceProvider struct {
	cfg         config.IndexerConfig
	allowedRoot string
}

// NewLocalSourceProvider creates a SourceProvider backed by the local filesystem.
func NewLocalSourceProvider(cfg config.IndexerConfig) services.SourceProvider {
	return &localSourceProvider{
		cfg:         cfg,
		allowedRoot: cfg.LocalAllowedRoot(),
	}
}

// NewLocalSourceProviderWithRoot creates a SourceProvider with an explicit allowed root.
func NewLocalSourceProviderWithRoot(root string) services.SourceProvider {
	return &localSourceProvider{
		allowedRoot: root,
	}
}

// validatePath ensures the resolved path is within the configured allowed root.
func (p *localSourceProvider) validatePath(path string) error {
	if p.allowedRoot == "" {
		return fmt.Errorf("local source provider: allowed root is not configured")
	}

	allowedAbs, err := filepath.Abs(p.allowedRoot)
	if err != nil {
		return fmt.Errorf("local source provider: invalid allowed root: %w", err)
	}

	cleaned, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("local source provider: invalid path: %w", err)
	}

	// Resolve symlinks to prevent symlink-based traversal.
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		cleaned = resolved
	}
	if resolved, err := filepath.EvalSymlinks(allowedAbs); err == nil {
		allowedAbs = resolved
	}

	if cleaned != allowedAbs && !strings.HasPrefix(cleaned, allowedAbs+string(os.PathSeparator)) {
		return fmt.Errorf("local source provider: path %q is outside allowed root %q", path, p.allowedRoot)
	}

	return nil
}

func (p *localSourceProvider) ListFiles(ctx context.Context, root string) ([]services.FileInfo, error) {
	// Pipeline.Index passes "" to BuildTree expecting the provider to use
	// its configured root (the comment in pipeline.go:206 spells this out).
	// LocalSourceProvider historically treated "" as a literal path which
	// blew up validation — rewrite it to allowedRoot so the legacy flow
	// works against a single configured repo. Not safe for multi-repo use.
	if root == "" {
		root = p.allowedRoot
	}
	if err := p.validatePath(root); err != nil {
		return nil, err
	}

	skipDirs := make(map[string]bool, len(DefaultSkipDirs))
	for k, v := range DefaultSkipDirs {
		skipDirs[k] = v
	}
	if p.cfg != nil {
		for _, d := range p.cfg.SkipDirs() {
			skipDirs[d] = true
		}
	}

	var files []services.FileInfo
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(root, path)
		files = append(files, services.FileInfo{
			Path:         path,
			RelativePath: relPath,
			Size:         info.Size(),
			IsDir:        false,
		})
		return nil
	})
	return files, err
}

func (p *localSourceProvider) ReadFile(_ context.Context, path string) ([]byte, error) {
	if err := p.validatePath(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (p *localSourceProvider) GetFileHash(_ context.Context, path string) (string, error) {
	if err := p.validatePath(path); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

func (p *localSourceProvider) Type() string { return "local" }
func (p *localSourceProvider) Close() error { return nil }
