package dto

type SearchRequest struct {
	Query       string `json:"query" binding:"required"`
	RepoPath    string `json:"repo_path" binding:"required"`
	Strategy    string `json:"strategy"` // "multi" | "vector" | "keyword" | "graph" | "hybrid"
	Limit       int    `json:"limit"`    // default 10
	UserID      string `json:"user_id"`
	WorkspaceID uint64 `json:"workspace_id"`
	MachineID   string `json:"machine_id"`
	RepoURL     string `json:"repo_url"`
	GithubOrgID string `json:"github_org_id"`
}
