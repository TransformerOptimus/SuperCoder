package request

type CreatePRFromCodeEditorRequest struct {
	ProjectID   int `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}