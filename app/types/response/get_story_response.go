package response

type GetStoryResponse struct {
	StoryId    int    `json:"story_id"`
	StoryTitle string `json:"story_title"`
}
