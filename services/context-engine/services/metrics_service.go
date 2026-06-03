package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// modelPricing maps model name prefixes to per-million-token pricing (USD).
// Order matters — first prefix match wins. More specific prefixes go first.
// Sources: https://developers.openai.com/api/docs/pricing
//          https://docs.anthropic.com/en/docs/about-claude/pricing
var modelPricing = []struct {
	prefix string
	input  float64
	output float64
}{
	// OpenAI — specific variants before base names
	{"gpt-5.4-pro", 30.00, 180.00},
	{"gpt-5.4-nano", 0.20, 1.25},
	{"gpt-5.4-mini", 0.75, 4.50},
	{"gpt-5.4", 2.50, 15.00},
	{"gpt-5.3-codex", 1.75, 14.00},
	{"gpt-5.3", 1.75, 14.00},
	{"gpt-5-codex", 1.25, 10.00},
	{"gpt-5-mini", 0.25, 2.00},
	{"gpt-5-nano", 0.10, 0.40},
	{"gpt-5", 1.25, 10.00},
	{"o4-mini", 1.10, 4.40},
	{"o3", 2.00, 8.00},
	// Anthropic
	{"claude-opus-4.6", 5.00, 25.00},
	{"claude-opus-4.5", 5.00, 25.00},
	{"claude-opus-4.1", 15.00, 75.00},
	{"claude-opus-4", 15.00, 75.00},
	{"claude-sonnet-4", 3.00, 15.00},
	{"claude-haiku-4.5", 1.00, 5.00},
	{"claude-haiku-3.5", 0.80, 4.00},
	{"claude-haiku-3", 0.25, 1.25},
}

// LookupModelPricing returns (inputPricePerMillion, outputPricePerMillion) for a model.
// Falls back to claude-sonnet pricing if unknown.
// LookupModelPricing returns (inputPricePerMillion, outputPricePerMillion).
// Second return indicates whether the model was found in the pricing table.
func LookupModelPricing(model string) (float64, float64, bool) {
	lower := strings.ToLower(model)
	for _, p := range modelPricing {
		if strings.HasPrefix(lower, p.prefix) {
			return p.input, p.output, true
		}
	}
	return 3.0, 15.0, false
}

// ReviewMetrics tracks metrics for a code review session.
type ReviewMetrics struct {
	StartTime             time.Time            `json:"start_time"`
	EndTime               time.Time            `json:"end_time"`
	TotalDuration         time.Duration        `json:"total_duration_ms"`
	FilesReviewed         int                  `json:"files_reviewed"`
	InputTokens           int                  `json:"input_tokens"`
	OutputTokens          int                  `json:"output_tokens"`
	CacheCreationTokens   int                  `json:"cache_creation_tokens"`
	CacheReadTokens       int                  `json:"cache_read_tokens"`
	TotalTokens           int                  `json:"total_tokens"`
	EstimatedCostUSD      float64              `json:"estimated_cost_usd"`
	PricingFallback       bool                 `json:"pricing_fallback,omitempty"` // true if model not in pricing table
	EstimatedCacheSavings float64              `json:"estimated_cache_savings_usd"`
	Model                 string               `json:"model"`
	ToolExecutions        map[string]ToolStats `json:"tool_executions"`
	EmbeddingMetrics      EmbeddingMetrics     `json:"embedding_metrics"`
}

type ToolStats struct {
	ExecutionCount  int           `json:"execution_count"`
	TotalDuration   time.Duration `json:"total_duration_ms"`
	AverageDuration time.Duration `json:"average_duration_ms"`
}

type EmbeddingMetrics struct {
	SearchQueries      int           `json:"search_queries"`
	SearchResultsTotal int           `json:"search_results_total"`
	EmbeddingTime      time.Duration `json:"embedding_time_ms"`
	EmbeddingQuality   float64       `json:"quality_score"`
	QualityDescription string        `json:"quality_description"`
}

type IndexMetrics struct {
	StartTime           time.Time     `json:"start_time"`
	EndTime             time.Time     `json:"end_time"`
	TotalDuration       time.Duration `json:"total_duration_ms"`
	FilesScanned        int           `json:"files_scanned"`
	FilesIndexed        int           `json:"files_indexed"`
	ElementsParsed      int           `json:"elements_parsed"`
	ChunksCreated       int           `json:"chunks_created"`
	EmbeddingsGenerated int           `json:"embeddings_generated"`
	EmbeddingTime       time.Duration `json:"embedding_time_ms"`
	AverageTimePerFile  time.Duration `json:"average_time_per_file_ms"`
}

type Timer struct {
	start time.Time
}

func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

func (t *Timer) Elapsed() time.Duration {
	return time.Since(t.start)
}

func NewReviewMetrics() *ReviewMetrics {
	return &ReviewMetrics{
		StartTime:      time.Now(),
		ToolExecutions: make(map[string]ToolStats),
	}
}

// ModelPricing holds per-million-token pricing (input, output) for cost estimation.
type ModelPricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

// EstimateCostForModel looks up pricing by model name and estimates cost.
func (m *ReviewMetrics) EstimateCostForModel(model string) {
	input, output, found := LookupModelPricing(model)
	if !found {
		m.PricingFallback = true
	}
	m.EstimateCost(input, output)
}

func (m *ReviewMetrics) EstimateCost(inputPricePerMillion, outputPricePerMillion float64) {
	// Cache write tokens cost 1.25x input price, cache read tokens cost 0.10x input price.
	// Non-cached input tokens = InputTokens - CacheCreationTokens - CacheReadTokens.
	cacheWritePrice := inputPricePerMillion * 1.25
	cacheReadPrice := inputPricePerMillion * 0.10
	nonCachedInput := m.InputTokens - m.CacheCreationTokens - m.CacheReadTokens

	m.EstimatedCostUSD = (float64(nonCachedInput)/1_000_000)*inputPricePerMillion +
		(float64(m.CacheCreationTokens)/1_000_000)*cacheWritePrice +
		(float64(m.CacheReadTokens)/1_000_000)*cacheReadPrice +
		(float64(m.OutputTokens)/1_000_000)*outputPricePerMillion

	// What the cost would have been without caching.
	costWithout := (float64(m.InputTokens)/1_000_000)*inputPricePerMillion +
		(float64(m.OutputTokens)/1_000_000)*outputPricePerMillion
	m.EstimatedCacheSavings = costWithout - m.EstimatedCostUSD
}

func (m *ReviewMetrics) Complete() {
	m.EndTime = time.Now()
	m.TotalDuration = m.EndTime.Sub(m.StartTime)
	m.TotalTokens = m.InputTokens + m.OutputTokens

	if m.EmbeddingMetrics.SearchQueries > 0 {
		avgResultsPerQuery := float64(m.EmbeddingMetrics.SearchResultsTotal) / float64(m.EmbeddingMetrics.SearchQueries)
		// Quality: did searches return any results? 1+ avg = good, 2+ = excellent.
		m.EmbeddingMetrics.EmbeddingQuality = avgResultsPerQuery / 3.0
		if m.EmbeddingMetrics.EmbeddingQuality > 1.0 {
			m.EmbeddingMetrics.EmbeddingQuality = 1.0
		}

		switch {
		case m.EmbeddingMetrics.SearchResultsTotal == 0:
			m.EmbeddingMetrics.QualityDescription = "no results - index may be empty or queries too narrow"
		case avgResultsPerQuery < 1.0:
			m.EmbeddingMetrics.QualityDescription = "sparse - some queries returned no results"
		case avgResultsPerQuery < 2.0:
			m.EmbeddingMetrics.QualityDescription = "targeted - focused results returned"
		case avgResultsPerQuery < 3.0:
			m.EmbeddingMetrics.QualityDescription = "good - consistent results"
		default:
			m.EmbeddingMetrics.QualityDescription = "rich - broad context retrieved"
		}
	}
}

func (m *ReviewMetrics) RecordToolExecution(toolName string, duration time.Duration) {
	stats := m.ToolExecutions[toolName]
	stats.ExecutionCount++
	stats.TotalDuration += duration
	stats.AverageDuration = stats.TotalDuration / time.Duration(stats.ExecutionCount)
	m.ToolExecutions[toolName] = stats
}

func (m *ReviewMetrics) RecordSearch(resultsCount int, duration time.Duration) {
	m.EmbeddingMetrics.SearchQueries++
	m.EmbeddingMetrics.SearchResultsTotal += resultsCount
	m.EmbeddingMetrics.EmbeddingTime += duration
}

func (m *ReviewMetrics) RecordTokens(inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int) {
	m.InputTokens += inputTokens
	m.OutputTokens += outputTokens
	m.CacheCreationTokens += cacheCreationTokens
	m.CacheReadTokens += cacheReadTokens
}

func (m *ReviewMetrics) ToJSON() (string, error) {
	m.Complete()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (m *ReviewMetrics) String() string {
	m.Complete()
	result := fmt.Sprintf(`
=== Code Review Metrics Summary ===
Duration: %v
Files Reviewed: %d
Tokens Used: %d (input: %d, output: %d)
Cache: %d created, %d read (saved $%.4f)

Tool Executions:
`, m.TotalDuration, m.FilesReviewed, m.TotalTokens, m.InputTokens, m.OutputTokens,
		m.CacheCreationTokens, m.CacheReadTokens, m.EstimatedCacheSavings)

	for tool, stats := range m.ToolExecutions {
		result += fmt.Sprintf("  %s: %d calls, total: %v, avg: %v\n",
			tool, stats.ExecutionCount, stats.TotalDuration, stats.AverageDuration)
	}

	queries := m.EmbeddingMetrics.SearchQueries
	if queries == 0 {
		queries = 1
	}
	result += fmt.Sprintf(`
Embedding Metrics:
  Search Queries: %d
  Total Results: %d
  Avg per Query: %.1f
  Quality Score: %.2f (%s)
  Embedding Time: %v
=================================
`, m.EmbeddingMetrics.SearchQueries,
		m.EmbeddingMetrics.SearchResultsTotal,
		float64(m.EmbeddingMetrics.SearchResultsTotal)/float64(queries),
		m.EmbeddingMetrics.EmbeddingQuality,
		m.EmbeddingMetrics.QualityDescription,
		m.EmbeddingMetrics.EmbeddingTime)

	return result
}

func NewIndexMetrics() *IndexMetrics {
	return &IndexMetrics{
		StartTime: time.Now(),
	}
}

func (m *IndexMetrics) Complete() {
	m.EndTime = time.Now()
	m.TotalDuration = m.EndTime.Sub(m.StartTime)
	if m.FilesIndexed > 0 {
		m.AverageTimePerFile = m.TotalDuration / time.Duration(m.FilesIndexed)
	}
}

func (m *IndexMetrics) String() string {
	m.Complete()
	return fmt.Sprintf(`
=== Index Metrics Summary ===
Duration: %v
Files Scanned: %d
Files Indexed: %d
Elements Parsed: %d
Chunks Created: %d
Embeddings Generated: %d
Embedding Time: %v
Avg Time per File: %v
==============================
`, m.TotalDuration, m.FilesScanned, m.FilesIndexed, m.ElementsParsed,
		m.ChunksCreated, m.EmbeddingsGenerated, m.EmbeddingTime, m.AverageTimePerFile)
}
