package dto

type CreateJobRequest struct {
	ExecutionId   int64    `json:"executionId"`
	ProjectId     string   `json:"projectId"`
	StoryId       int64    `json:"storyId"`
	IsReExecution bool     `json:"isReExecution"`
	Branch        string   `json:"branch"`
	PullRequestId int64    `json:"pullRequestId"`
	ExecutorImage string   `json:"executorImage"`
	Env           []string `json:"env"`
}

type CreateJobResponse struct {
	JobId string `json:"jobId"`
}
