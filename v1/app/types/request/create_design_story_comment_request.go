package request

type CreateDesignStoryCommentRequest struct {
	StoryID uint   `json:"story_id"`
	Comment string `json:"comment"`
}
