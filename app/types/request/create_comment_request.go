package request

type CreateCommentRequest struct {
	PullRequestID uint   `json:"pull_request_id"`
	Comment       string `json:"comment"`
}
