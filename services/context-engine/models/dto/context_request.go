package dto

type ContextRequest struct {
	Query       string `json:"query" binding:"required"`
	RepoPath    string `json:"repo_path" binding:"required"`
	RepoURL     string `json:"repo_url"`
	Limit       int    `json:"limit"` // default 5
	UserID      string `json:"user_id"`
	WorkspaceID uint64 `json:"workspace_id"`
	MachineID   string `json:"machine_id"`
	GithubOrgID string `json:"github_org_id"`
	SourceType  string `json:"source_type"`
	S3Bucket    string `json:"s3_bucket"`
	S3Prefix    string `json:"s3_prefix"`
}
