package consumers

import (
	"errors"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// classifyExhaustionReason maps a retryable error that has exhausted its
// Asynq retry budget to a stable sub-code written into sync_batches /
// failed_files. It inspects the error chain for sentinels wrapped inside the
// pipeline at the known batch-level failure sites; anything that doesn't
// match a known sentinel falls back to the generic "retries_exhausted"
// bucket so today's coarse attribution still works.
//
// Sub-codes:
//
//	qdrant_upsert_exhausted   — services.ErrQdrantUpsert (Qdrant write failed)
//	openai_embed_exhausted    — services.ErrOpenAIEmbed (OpenAI embedding call failed)
//	parse_exhausted           — services.ErrTreeSitterParse (tree-sitter indexer failed)
//	retries_exhausted         — anything else
//
// The raw error is expected to be logged separately via zap.Error by the
// caller; this helper returns only the stable code for persistence.
func classifyExhaustionReason(err error) string {
	switch {
	case errors.Is(err, services.ErrQdrantUpsert):
		return "qdrant_upsert_exhausted"
	case errors.Is(err, services.ErrOpenAIEmbed):
		return "openai_embed_exhausted"
	case errors.Is(err, services.ErrTreeSitterParse):
		return "parse_exhausted"
	default:
		return "retries_exhausted"
	}
}
