package response

type GetDesignStoriesOfProjectId struct {
	StoryID           int    `json:"id"`
	StoryName         string `json:"title"`
	StoryInputFileURL string `json:"input_file_url"`
	StoryStatus       string `json:"status"`
	Reason            string `json:"reason"`
	CreatedAt         string `json:"created_on"`
	ReviewViewed      bool   `json:"review_viewed"`
	FrontendURL       string `json:"frontend_url"`
}
