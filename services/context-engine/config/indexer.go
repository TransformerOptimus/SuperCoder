package config

import (
	"github.com/knadh/koanf/v2"
)

type IndexerConfig interface {
	Enabled() bool
	StreamingEnabled() bool
	ChunkMaxTokens() int
	TokensPerChar() float64
	SkipDirs() []string
	SupportedLanguages() []string
	S3Bucket() string
	S3Prefix() string
	S3Region() string
	S3Endpoint() string
	S3AccessKey() string
	S3SecretKey() string
	MerkleDir() string
	MerkleS3Bucket() string
	MerkleS3Prefix() string
	MerkleS3Region() string
	SmartChunkingEnabled() bool
	ChunkMinMergeTokens() int
	LocalAllowedRoot() string
	EmbeddingConcurrency() int
}

type indexerConfigImpl struct {
	config *koanf.Koanf
}

func NewIndexerConfig(config *koanf.Koanf) IndexerConfig {
	return &indexerConfigImpl{config: config}
}

func (c *indexerConfigImpl) Enabled() bool {
	return c.config.Bool("indexer.enabled")
}

// StreamingEnabled gates the v3 streaming sync stack: the /diff,
// /stream, /sync-complete HTTP endpoints, the outbox dispatcher
// goroutine, the sync_sessions TTL GC, and the index:stream_batch /
// finalize:sync Asynq handlers. Requires Enabled() to also be true —
// the streaming providers live inside the indexer.enabled DI block.
func (c *indexerConfigImpl) StreamingEnabled() bool {
	return c.config.Bool("indexer.streaming.enabled")
}

func (c *indexerConfigImpl) ChunkMaxTokens() int {
	return c.config.Int("indexer.chunk.max.tokens")
}

func (c *indexerConfigImpl) TokensPerChar() float64 {
	return c.config.Float64("indexer.tokens.per.char")
}

func (c *indexerConfigImpl) SkipDirs() []string {
	return c.config.Strings("indexer.skip.dirs")
}

func (c *indexerConfigImpl) S3Bucket() string {
	return c.config.String("s3.bucket")
}

func (c *indexerConfigImpl) S3Prefix() string {
	return c.config.String("s3.prefix")
}

func (c *indexerConfigImpl) S3Region() string {
	return c.config.String("s3.region")
}

func (c *indexerConfigImpl) S3Endpoint() string {
	return c.config.String("s3.endpoint")
}

func (c *indexerConfigImpl) S3AccessKey() string {
	return c.config.String("s3.access.key")
}

func (c *indexerConfigImpl) S3SecretKey() string {
	return c.config.String("s3.secret.key")
}

func (c *indexerConfigImpl) MerkleDir() string {
	return c.config.String("merkle.dir")
}

func (c *indexerConfigImpl) MerkleS3Bucket() string {
	return c.config.String("merkle.s3.bucket")
}

func (c *indexerConfigImpl) MerkleS3Prefix() string {
	return c.config.String("merkle.s3.prefix")
}

func (c *indexerConfigImpl) MerkleS3Region() string {
	return c.config.String("merkle.s3.region")
}

func (c *indexerConfigImpl) SmartChunkingEnabled() bool {
	return c.config.Bool("indexer.chunk.smart")
}

func (c *indexerConfigImpl) ChunkMinMergeTokens() int {
	v := c.config.Int("indexer.chunk.min.merge.tokens")
	if v <= 0 {
		return 64
	}
	return v
}

func (c *indexerConfigImpl) LocalAllowedRoot() string {
	return c.config.String("indexer.local.allowed.root")
}

// EmbeddingConcurrency caps the number of in-flight OpenAI embed calls
// across the whole process. Tuned to stay under OpenAI rate limits while
// keeping the streaming pipeline saturated on typical repos. Default (5)
// is set in injection/default_config_provider.go; prod can tune via
// SUPERCODER_INDEXER_EMBEDDING_CONCURRENCY without a redeploy.
func (c *indexerConfigImpl) EmbeddingConcurrency() int {
	v := c.config.Int("indexer.embedding.concurrency")
	if v <= 0 {
		return 5
	}
	return v
}

func (c *indexerConfigImpl) SupportedLanguages() []string {
	return []string{
		"go", "python", "javascript", "typescript", "tsx",
		"java", "rust", "c", "cpp", "ruby",
		"starlark", "protobuf", "sql", "yaml", "markdown",
	}
}
