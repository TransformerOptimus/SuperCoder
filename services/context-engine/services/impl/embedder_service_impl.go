package impl

import (
	"context"
	"fmt"
	"sync"

	"github.com/pkoukk/tiktoken-go"
	tiktoken_loader "github.com/pkoukk/tiktoken-go-loader"
	"go.uber.org/zap"
	"resty.dev/v3"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

// OpenAI embeddings API hard limits (text-embedding-3-large/small):
//   - 300,000 tokens total per request
//   - 8,192 tokens per individual input
//   - 2,048 inputs per request
//
// We use exact BPE token counts via tiktoken-go's cl100k_base encoding
// and leave a small safety margin for any drift between pkoukk/tiktoken-go
// and OpenAI's server-side tokenizer.
const (
	// maxEmbedBatchTokens caps accumulated tokens per request.
	// 10K tokens of headroom (~3.3%) under OpenAI's 300K limit.
	maxEmbedBatchTokens = 290_000

	// maxEmbedBatchCount caps inputs per request (OpenAI documented hard limit).
	maxEmbedBatchCount = 2048

	// maxSingleInputTokens caps per-input length.
	// 192 tokens of headroom under OpenAI's 8,192 per-input limit.
	maxSingleInputTokens = 8000

	// embeddingEncoding is the BPE encoding used by text-embedding-3-large
	// and text-embedding-3-small. Hardcoded — do not derive from model name,
	// as EncodingForModel lags behind OpenAI releases.
	embeddingEncoding = "cl100k_base"
)

// bpeLoaderOnce ensures the offline BPE loader is registered exactly once
// per process before any tiktoken.GetEncoding call. The offline loader uses
// //go:embed'd vocab files so tokenizer initialization never touches the network.
var bpeLoaderOnce sync.Once

func registerOfflineBPELoader() {
	bpeLoaderOnce.Do(func() {
		tiktoken.SetBpeLoader(tiktoken_loader.NewOfflineLoader())
	})
}

type openAIEmbedderImpl struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	client     *resty.Client
	enc        *tiktoken.Tiktoken
	logger     *zap.Logger
}

func NewEmbedderService(cfg config.OpenAIConfig, logger *zap.Logger) (services.EmbedderService, error) {
	registerOfflineBPELoader()

	enc, err := tiktoken.GetEncoding(embeddingEncoding)
	if err != nil {
		return nil, fmt.Errorf("embedder: load %s encoding: %w", embeddingEncoding, err)
	}

	log := logger.Named("embedder")
	log.Info("Embedder initialized",
		zap.String("encoding", embeddingEncoding),
		zap.String("model", cfg.EmbeddingModel()),
		zap.Int("dimensions", cfg.EmbeddingDimensions()),
		zap.Int("max_batch_tokens", maxEmbedBatchTokens),
		zap.Int("max_batch_count", maxEmbedBatchCount),
		zap.Int("max_single_input_tokens", maxSingleInputTokens),
	)

	return &openAIEmbedderImpl{
		baseURL:    cfg.BaseURL(),
		apiKey:     cfg.APIKey(),
		model:      cfg.EmbeddingModel(),
		dimensions: cfg.EmbeddingDimensions(),
		client:     newRestyClient(cfg.APIKey()),
		enc:        enc,
		logger:     log,
	}, nil
}

func (e *openAIEmbedderImpl) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Pre-tokenize each input once. Truncate any text exceeding the per-input
	// limit at a token boundary (not a byte boundary) so UTF-8 stays valid.
	// Use copy-on-write so we never mutate the caller's slice.
	working := texts
	cloned := false
	tokenCounts := make([]int, len(texts))
	truncatedCount := 0
	totalTokens := 0

	for i, t := range texts {
		ids := e.enc.Encode(t, nil, nil)
		n := len(ids)
		if n > maxSingleInputTokens {
			if !cloned {
				working = append([]string(nil), texts...)
				cloned = true
			}
			e.logger.Warn("Truncating oversized embedding input",
				zap.Int("index", i),
				zap.Int("original_tokens", n),
				zap.Int("truncated_tokens", maxSingleInputTokens),
			)
			ids = ids[:maxSingleInputTokens]
			working[i] = e.enc.Decode(ids)
			truncatedCount++
		}
		tokenCounts[i] = min(n, maxSingleInputTokens)
		totalTokens += tokenCounts[i]
	}

	e.logger.Debug("Embed request received",
		zap.Int("input_count", len(texts)),
		zap.Int("total_tokens", totalTokens),
		zap.Int("truncated_inputs", truncatedCount),
	)

	allVectors := make([][]float32, len(working))

	// Batch by accumulated token budget and hard item-count cap.
	batchStart := 0
	batchTokens := 0
	batchNum := 0
	for i := 0; i < len(working); i++ {
		n := tokenCounts[i]

		// If adding this item would exceed either limit, flush the current batch first.
		// Always include at least one item per batch to guarantee progress.
		if i > batchStart && (batchTokens+n > maxEmbedBatchTokens || i-batchStart >= maxEmbedBatchCount) {
			batchNum++
			if err := e.embedAndCopy(ctx, working, allVectors, batchStart, i, batchNum, batchTokens); err != nil {
				return nil, err
			}
			batchStart = i
			batchTokens = 0
		}
		batchTokens += n
	}

	// Flush the final batch.
	if batchStart < len(working) {
		batchNum++
		if err := e.embedAndCopy(ctx, working, allVectors, batchStart, len(working), batchNum, batchTokens); err != nil {
			return nil, err
		}
	}

	// At INFO only when we had to split across multiple batches — that's the
	// interesting signal (workload was large enough to exercise the splitter).
	// Single-batch calls are the common case and stay at DEBUG.
	if batchNum > 1 {
		e.logger.Info("Embed request split across batches",
			zap.Int("input_count", len(texts)),
			zap.Int("total_tokens", totalTokens),
			zap.Int("batches_sent", batchNum),
		)
	} else {
		e.logger.Debug("Embed request completed",
			zap.Int("input_count", len(texts)),
			zap.Int("total_tokens", totalTokens),
			zap.Int("batches_sent", batchNum),
		)
	}

	return allVectors, nil
}

// embedAndCopy embeds texts[start:end] and copies the result into allVectors[start:end].
func (e *openAIEmbedderImpl) embedAndCopy(ctx context.Context, texts []string, allVectors [][]float32, start, end, batchNum, batchTokens int) error {
	batch := texts[start:end]
	e.logger.Debug("Sending embedding batch",
		zap.Int("batch_num", batchNum),
		zap.Int("start", start),
		zap.Int("end", end),
		zap.Int("count", len(batch)),
		zap.Int("tokens", batchTokens),
	)
	vecs, err := e.embedBatch(ctx, batch)
	if err != nil {
		return fmt.Errorf("embedding batch %d (%d-%d, %d tokens) failed: %w", batchNum, start, end, batchTokens, err)
	}
	copy(allVectors[start:end], vecs)
	return nil
}

func (e *openAIEmbedderImpl) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	type embeddingRequest struct {
		Input      []string `json:"input"`
		Model      string   `json:"model"`
		Dimensions int      `json:"dimensions,omitempty"`
	}

	type embeddingData struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	}

	type embeddingResponse struct {
		Data  []embeddingData `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	reqBody := embeddingRequest{
		Input: texts,
		Model: e.model,
	}
	if e.dimensions > 0 {
		reqBody.Dimensions = e.dimensions
	}

	var result embeddingResponse

	resp, err := e.client.R().
		SetContext(ctx).
		SetBody(reqBody).
		SetResult(&result).
		Post(e.baseURL + "/embeddings")

	if err != nil {
		return nil, fmt.Errorf("openai embedding request failed: %w", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("openai embedding API error: %s — %s", resp.Status(), resp.String())
	}

	if result.Error != nil {
		return nil, fmt.Errorf("openai embedding API error: %s — %s", resp.Status(), result.Error.Message)
	}

	vectors := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < 0 || d.Index >= len(vectors) {
			return nil, fmt.Errorf("embedding response index %d out of range [0, %d)", d.Index, len(vectors))
		}
		vectors[d.Index] = d.Embedding
	}

	for i, v := range vectors {
		if len(v) == 0 {
			return nil, services.Terminal("empty_embedding", fmt.Errorf("embedding response returned empty vector at index %d (got %d of %d responses)", i, len(result.Data), len(texts)))
		}
	}

	dims := 0
	if len(vectors) > 0 {
		dims = len(vectors[0])
	}
	e.logger.Debug("Generated embeddings",
		zap.Int("count", len(vectors)),
		zap.Int("dimensions", dims))

	return vectors, nil
}

func (e *openAIEmbedderImpl) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	vecs, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) > 0 {
		return vecs[0], nil
	}
	return nil, nil
}
