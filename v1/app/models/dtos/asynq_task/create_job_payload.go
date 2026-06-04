package asynq_task

type CreateJobPayload struct {
	StoryID       uint
	ReExecute     bool
	PullRequestId uint
}
