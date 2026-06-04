package dto

type ReviewRequest struct {
	PRURL       string `json:"pr_url" binding:"required"`
	Diff        string `json:"diff" binding:"required"`
	Description string `json:"description"`
	RepoID      string `json:"repo_id" binding:"required"`
	GithubOrgID string `json:"github_org_id"`
	UserID      string `json:"user_id"`
	WorkspaceID uint64 `json:"workspace_id"`
	MachineID   string `json:"machine_id"`
	RepoDBID    uint   `json:"repo_db_id"`

	// IsTrivial indicates the gatekeeper classified this PR as trivial
	// (test-only or small change). Graph context is skipped.
	IsTrivial bool `json:"-"` // server-side only, not from API

	// S3 archive location — set after successful archive so the pipeline
	// can read full file contents for changed files.
	S3Bucket string `json:"s3_bucket,omitempty"`
	S3Prefix string `json:"s3_prefix,omitempty"`

	// Files contains per-file patches for chunked review. Populated by the
	// review orchestrator, not by the API caller.
	Files []PRFile `json:"files,omitempty"`

	// ExistingComments are previously raised unresolved comments on this PR.
	// Passed through so the LLM can avoid duplicates and resolve fixed issues.
	ExistingComments []ExistingComment `json:"existing_comments,omitempty"`
}

// ExistingComment mirrors services.ExistingComment for the DTO layer.
type ExistingComment struct {
	ID         string `json:"id"`
	FilePath   string `json:"file_path"`
	LineNumber int32  `json:"line_number"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Body       string `json:"body"`
}

// PRFile represents a single file's diff in a pull request.
type PRFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int32  `json:"additions"`
	Deletions int32  `json:"deletions"`
	Patch     string `json:"patch"`
}
