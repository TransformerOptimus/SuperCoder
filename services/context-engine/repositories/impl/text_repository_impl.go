package impl

import (
	"context"
	"fmt"
	"strings"

	"github.com/qdrant/go-client/qdrant"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type textRepositoryImpl struct {
	client *qdrant.Client
	logger *zap.Logger
}

// NewTextSearchRepository creates a Qdrant-backed text search repository
// using Qdrant's full-text payload index on the content field.
func NewTextSearchRepository(cfg config.QdrantConfig, logger *zap.Logger) (repositories.TextSearchRepository, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: cfg.Host(),
		Port: cfg.Port(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant for text search: %w", err)
	}
	return &textRepositoryImpl{
		client: client,
		logger: logger.Named("text-repo"),
	}, nil
}

func (t *textRepositoryImpl) EnsureIndex(ctx context.Context, collection string) error {
	fieldType := qdrant.FieldType_FieldTypeText
	if _, err := t.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: collection,
		FieldName:      "content",
		FieldType:      &fieldType,
		FieldIndexParams: qdrant.NewPayloadIndexParamsText(&qdrant.TextIndexParams{
			Tokenizer:   qdrant.TokenizerType_Word,
			MinTokenLen: qdrant.PtrOf(uint64(2)),
			MaxTokenLen: qdrant.PtrOf(uint64(40)),
			Lowercase:   qdrant.PtrOf(true),
		}),
		Wait: qdrant.PtrOf(true),
	}); err != nil {
		t.logger.Debug("Text index creation note",
			zap.String("collection", collection),
			zap.Error(err))
	}
	return nil
}

// IndexChunks is a no-op because VectorRepository already upserts the chunks
// (including the content field) into Qdrant before this is called.
func (t *textRepositoryImpl) IndexChunks(_ context.Context, _ string, _ []services.CodeChunk) error {
	return nil
}

// stopwords that rarely help in code search.
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "can": true, "shall": true,
	"how": true, "what": true, "when": true, "where": true, "which": true,
	"who": true, "whom": true, "why": true,
	"it": true, "its": true, "this": true, "that": true, "these": true,
	"those": true, "i": true, "we": true, "you": true, "he": true,
	"she": true, "they": true, "me": true, "my": true, "your": true,
	"of": true, "in": true, "to": true, "for": true, "with": true,
	"on": true, "at": true, "by": true, "from": true, "up": true,
	"about": true, "into": true, "through": true, "and": true, "but": true,
	"or": true, "not": true, "no": true, "if": true, "then": true,
}

func (t *textRepositoryImpl) Search(ctx context.Context, collection string, query string, limit int, filter services.SearchFilter) ([]repositories.TextSearchResult, error) {
	// Split query into meaningful tokens (strip stopwords).
	tokens := extractSearchTokens(query)
	if len(tokens) == 0 {
		return nil, nil
	}

	// Build isolation filters (applied as must conditions).
	var isolationConditions []*qdrant.Condition
	if filter.OrgID != "" {
		isolationConditions = append(isolationConditions, qdrant.NewMatch("github_org_id", filter.OrgID))
	}
	if filter.WorkspaceID != "" {
		isolationConditions = append(isolationConditions, qdrant.NewMatch("workspace_id", filter.WorkspaceID))
	}
	if filter.UserID != "" {
		isolationConditions = append(isolationConditions, qdrant.NewMatch("user_id", filter.UserID))
	}
	if filter.MachineID != "" {
		isolationConditions = append(isolationConditions, qdrant.NewMatch("machine_id", filter.MachineID))
	}
	if filter.RepoID > 0 {
		isolationConditions = append(isolationConditions, qdrant.NewMatchInt("repo_id", int64(filter.RepoID)))
	}

	// Use OR (Should) matching across tokens so any token can match,
	// instead of AND (Must) which requires ALL tokens in one chunk.
	var textConditions []*qdrant.Condition
	for _, token := range tokens {
		textConditions = append(textConditions, qdrant.NewMatchText("content", token))
	}

	// Combine: must match isolation filters AND should match at least one token.
	mustConditions := make([]*qdrant.Condition, len(isolationConditions))
	copy(mustConditions, isolationConditions)

	// Wrap text conditions in a Should (OR) filter.
	mustConditions = append(mustConditions, &qdrant.Condition{
		ConditionOneOf: &qdrant.Condition_Filter{
			Filter: &qdrant.Filter{
				Should: textConditions,
			},
		},
	})

	points, err := t.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collection,
		Filter: &qdrant.Filter{
			Must: mustConditions,
		},
		Limit:       qdrant.PtrOf(uint32(limit)),
		WithPayload: qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant text search failed: %w", err)
	}

	// Score by number of matching tokens (simple term frequency).
	// Require at least 1 token match to return a result.
	minMatches := 1

	var results []repositories.TextSearchResult
	for _, point := range points {
		content := point.Payload["content"].GetStringValue()
		contentLower := strings.ToLower(content)
		matchCount := 0
		for _, token := range tokens {
			if strings.Contains(contentLower, token) {
				matchCount++
			}
		}

		if matchCount < minMatches {
			continue
		}

		score := float64(matchCount) / float64(len(tokens))

		results = append(results, repositories.TextSearchResult{
			ChunkID:  point.Payload["chunk_id"].GetStringValue(),
			Content:  content,
			FilePath: point.Payload["file_path"].GetStringValue(),
			Language: point.Payload["language"].GetStringValue(),
			Score:    score,
		})
	}
	return results, nil
}

// extractSearchTokens splits query into lowercase tokens, removes stopwords and short tokens.
func extractSearchTokens(query string) []string {
	words := strings.Fields(strings.ToLower(query))
	var tokens []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}") // strip punctuation
		if len(w) < 2 || stopwords[w] {
			continue
		}
		tokens = append(tokens, w)
	}
	return tokens
}

// DeleteByFilePath is a no-op — VectorRepository handles point deletion.
func (t *textRepositoryImpl) DeleteByFilePath(_ context.Context, _ string, _ string) error {
	return nil
}

// DeleteByRepoID is a no-op — VectorRepository handles point deletion.
func (t *textRepositoryImpl) DeleteByRepoID(_ context.Context, _ string, _ uint) error {
	return nil
}

// DeleteIndex is a no-op — VectorRepository handles collection deletion.
func (t *textRepositoryImpl) DeleteIndex(_ context.Context, _ string) error {
	return nil
}
