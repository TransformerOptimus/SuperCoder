package services

import "context"

// Reviewer abstracts the LLM-powered code review backend.
// Implementations exist for different model providers (Anthropic, OpenAI, etc.).
type Reviewer interface {
	Review(ctx context.Context, input ReviewInput) (*ReviewOutput, error)
}

// ExistingComment represents a previously raised review comment that is still unresolved.
type ExistingComment struct {
	ID         string
	FilePath   string
	LineNumber int32
	Severity   string
	Category   string
	Body       string
}

// ReviewInput contains the data needed for a single review run.
type ReviewInput struct {
	Diff             string
	Files            []PRFileInfo
	Description      string
	GraphContext     string            // pre-assembled callers / callees / importers block
	FileContents     map[string]string // changed-file source code
	CalleeBodies     map[string]string // first-hop callee source ("file:fn" → body)
	SimilarCode      string            // similar-existing-code block for duplication detection
	WorkspaceID      uint64
	Provider         string
	Repository       string
	HeadRef          string
	ExistingComments []ExistingComment
}

// ReviewParams contains the parameters for initiating a code review.
type ReviewParams struct {
	WorkspaceID  uint64
	Provider     string
	Repository   string
	PRNumber     int
	Description  string
	HeadRef      string
	AuthorHandle string
}

// ReviewOutput is the structured result returned by the LLM.
type ReviewOutput struct {
	Verdict            string        `json:"verdict"`
	RiskLevel          string        `json:"risk_level"`
	RiskReason         string        `json:"risk_reason"`
	RiskScore          float64       `json:"risk_score,omitempty"`
	RiskFactors        []RiskFactor  `json:"risk_factors,omitempty"`
	Overview           string        `json:"overview"`
	AreasAffected      string        `json:"areas_affected"`
	Issues             []ReviewIssue `json:"issues"`
	ResolvedCommentIDs []string      `json:"resolved_comment_ids,omitempty"`
}

// RiskFactor is one component of the deterministic risk score.
type RiskFactor struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Value  float64 `json:"value"`
	Detail string  `json:"detail"`
}

// ReviewIssue represents a single code review finding.
type ReviewIssue struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	LineContent string `json:"line_content,omitempty"` // verbatim content the model cited for anchor verification
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Pattern     string `json:"pattern,omitempty"` // grouping key for dedup (e.g. "sql-injection")
	Description string `json:"description"`
	Problem     string `json:"problem"`
	Fix         string `json:"fix,omitempty"`
	UsesContext bool   `json:"uses_context,omitempty"` // true when graph_context / similar_code / full_file_contents informed the finding
}
