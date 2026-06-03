package consumers

const (
	TypeReviewPR  = "codereview:review_pr"
	TypeIndexRepo = "codereview:index_repo"
	TypeAuditRepo = "codereview:audit_repo"
)

type ReviewPRPayload struct {
	PRURL        string `json:"pr_url"`
	Diff         string `json:"diff"`
	Description  string `json:"description"`
	RepoID       string `json:"repo_id"`
	UserID       string `json:"user_id"`
	WorkspaceID  uint64 `json:"workspace_id"`
	MachineID    string `json:"machine_id"`
	Provider     string `json:"provider,omitempty"`
	Repository   string `json:"repository,omitempty"`
	PRNumber     int    `json:"pr_number,omitempty"`
	HeadRef      string `json:"head_ref,omitempty"`
	AuthorHandle string `json:"author_handle,omitempty"`
}

type IndexRepoPayload struct {
	RepoPath    string `json:"repo_path"`
	RepoURL     string `json:"repo_url"`
	Reindex     bool   `json:"reindex"`
	UserID      string `json:"user_id"`
	WorkspaceID uint64 `json:"workspace_id"`
	MachineID   string `json:"machine_id"`
	SourceType  string `json:"source_type,omitempty"`
	S3Bucket    string `json:"s3_bucket,omitempty"`
	S3Prefix    string `json:"s3_prefix,omitempty"`
}

type AuditRepoPayload struct {
	RepoPath string `json:"repo_path"`
}
