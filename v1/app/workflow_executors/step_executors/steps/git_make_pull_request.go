package steps

type GitMakePullRequestStep struct {
	BaseStep
	WorkflowStep
	Type          string
	PullRequestID uint
}

func (s GitMakePullRequestStep) StepType() string {
	return GIT.String()
}

func (s GitMakePullRequestStep) StepName() string {
	return GIT_CREATE_PULL_REQUEST_STEP.String()
}

func (s *GitMakePullRequestStep) WithPullRequestID(pullRequestID uint) {
	s.PullRequestID = pullRequestID
}
