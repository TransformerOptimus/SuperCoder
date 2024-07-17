package response

type GetStoryByIdResponse struct {
	Overview          StoryOverview `json:"overview"`
	TestCases         []string      `json:"test_cases"`
	Instructions      []string      `json:"instructions"`
	Status            string        `json:"status"`
	Reason            string        `json:"reason"`
	StoryInputFileUrl string        `json:"story_input_file_url"`
}

type StoryOverview struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
