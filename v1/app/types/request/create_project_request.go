package request

type CreateProjectRequest struct {
	Name              string `json:"name"`
	Framework         string `json:"framework"`
	FrontendFramework string `json:"frontend_framework"`
	Description       string `json:"description"`
}
