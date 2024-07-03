package workflow_executors

type WorkflowExecutionArgs struct {
	ExecutionId   int64
	StoryId       int64
	IsReExecution bool
	Branch        string
	PullRequestId int64
}
