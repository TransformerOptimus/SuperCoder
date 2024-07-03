package response

type GetAllProjectsResponse struct {
	ProjectId          uint   `json:"project_id"`
	ProjectName        string `json:"project_name"`
	ProjectDescription string `json:"project_description"`
	ProjectHashID      string `json:"project_hash_id"`
	ProjectUrl         string `json:"project_url"`
	ProjectBackendURL  string `json:"project_backend_url"`
	ProjectFrontendURL string `json:"project_frontend_url"`
	PullRequestCount   int    `json:"pull_request_count"`
}
