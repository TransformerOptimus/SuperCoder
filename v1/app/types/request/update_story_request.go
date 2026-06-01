package request

type UpdateStoryRequest struct {
	StoryID      int      `json:"story_id"`
	Summary      string   `json:"summary"`
	Description  string   `json:"description"`
	TestCases    []string `json:"test_cases"`
	Instructions string   `json:"instructions"`
}
