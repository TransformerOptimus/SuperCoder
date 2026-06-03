package services

import "errors"

// Sentinel errors wrapped at retryable batch-level failure sites inside the
// pipeline. The stream_batch consumer uses errors.Is on these to attribute an
// exhausted batch to a stable sub-code (qdrant_upsert_exhausted /
// openai_embed_exhausted / parse_exhausted) rather than the generic
// retries_exhausted bucket. Each sentinel's text matches the previous
// fmt.Errorf prefix so log output is unchanged.
var (
	ErrQdrantUpsert    = errors.New("qdrant upsert")
	ErrOpenAIEmbed     = errors.New("embed batch")
	ErrTreeSitterParse = errors.New("tree-sitter index")
)
