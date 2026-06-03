package dto

import "time"

type ReviewResponse struct {
	ID           string                  `json:"id"`
	RepoID       string                  `json:"repo_id"`
	PRNumber     int                     `json:"pr_number"`
	PRURL        string                  `json:"pr_url"`
	AuthorHandle string                  `json:"author_handle,omitempty"`
	Verdict      string                  `json:"verdict"`
	Comments     []ReviewCommentResponse `json:"comments"`
	Metrics      ReviewMetrics           `json:"metrics"`
	RawOutput    string                  `json:"raw_output,omitempty"`
	CreatedAt    time.Time               `json:"created_at"`
}

type ReviewCommentResponse struct {
	FilePath   string `json:"file_path"`
	LineNumber *int   `json:"line_number,omitempty"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Body       string `json:"body"`
}

type ReviewMetrics struct {
	CommentCount   int            `json:"comment_count"`
	SeverityCounts map[string]int `json:"severity_counts"`
	CategoryCounts map[string]int `json:"category_counts"`
}
