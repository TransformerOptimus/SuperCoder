package request

type RetrieveCodeRequest struct {
	FileName  string `json:"file_name"`
	ProjectID uint   `json:"project_id"`
	StoryID   uint   `json:"story_id"`
}