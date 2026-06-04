package dto

type GraphQueryRequest struct {
	Query        string `json:"query"`         // Free-text search query — finds functions via text+vector, then expands graph
	FunctionName string `json:"function_name"` // Direct function name lookup (optional if query is provided)
	FilePath     string `json:"file_path"`
	RepoPath     string `json:"repo_path" binding:"required"`
	QueryType    string `json:"query_type"` // "blast_radius" | "dependencies" | "" (default: both)
	UserID       string `json:"user_id"`
	WorkspaceID  uint64 `json:"workspace_id"`
	MachineID    string `json:"machine_id"`
	GithubOrgID  string `json:"github_org_id"`
}
