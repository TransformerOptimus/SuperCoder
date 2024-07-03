package dto

type CreateJobRequest struct {
	ExecutionId   int64    `json:"executionId"`
	StoryId       int64    `json:"storyId"`
	ProjectId     string   `json:"projectId"`
	IsReExecution bool     `json:"isReExecution"`
	Branch        string   `json:"branch"`
	PullRequestId int64    `json:"pullRequestId"`
	Env           []string `json:"env"`
}

type CreateJobResponse struct {
	JobId string `json:"jobId"`
}
