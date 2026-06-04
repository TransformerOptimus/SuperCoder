package request

type UpdateStoryStatusRequest struct {
	StoryId     int    `json:"story_id"`
	StoryStatus string `json:"story_status"`
}
