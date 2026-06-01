package response

type GetAllPullRequests struct {
	PullRequestID          int    `json:"pull_request_id"`
	PullRequestDescription string `json:"pull_request_description"`
	PullRequestName        string `json:"pull_request_name"`
	PullRequestNumber      int    `json:"pull_request_number"`
	CreatedOn              string `json:"created_on"`
	TotalComments          int64  `json:"total_comments"`
	Status                 string `json:"status"`
	MergedOn               string `json:"merged_on"`
	ClosedOn               string `json:"closed_on"`
}
