package dto

type IndexRequest struct {
	RepoPath    string `json:"repo_path" binding:"required"`
	RepoURL     string `json:"repo_url"`
	Reindex     bool   `json:"reindex"` // Force full reindex even if already indexed
	GithubOrgID string `json:"github_org_id"`
	UserID      string `json:"user_id"`
	WorkspaceID uint64 `json:"workspace_id"`
	MachineID   string `json:"machine_id"`
	SourceType  string `json:"source_type"` // "local" | "s3" | "github" (default: inferred)
	S3Bucket    string `json:"s3_bucket"`
	S3Prefix    string `json:"s3_prefix"`
}
