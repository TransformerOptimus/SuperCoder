package request

type CreateProjectRequest struct {
	Name        string `json:"name"`
	Framework   string `json:"framework"`
	Description string `json:"description"`
}
