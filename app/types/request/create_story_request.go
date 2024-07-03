package request

type CreateStoryRequest struct {
	ProjectId    int      `json:"project_id"`
	Summary      string   `json:"summary"`
	Description  string   `json:"description"`
	TestCases    []string `json:"test_cases"`
	Instructions string   `json:"instructions"`
}
