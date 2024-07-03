package response

type GetAllStoriesByProjectIDResponse struct {
	Todo       []StoryData `json:"TODO"`
	InProgress []StoryData `json:"IN_PROGRESS"`
	Done       []StoryData `json:"DONE"`
	InReview   []StoryData `json:"IN_REVIEW"`
}

type StoryData struct {
	StoryID   int    `json:"story_id"`
	StoryName string `json:"story_name"`
}
