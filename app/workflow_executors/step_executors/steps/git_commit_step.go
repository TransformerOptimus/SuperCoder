package steps

type GitCommitStep struct {
	BaseStep
	WorkflowStep
	Type string
}

func (s GitCommitStep) StepType() string {
	return GIT.String()
}

func (s GitCommitStep) StepName() string {
	return GIT_COMMIT_STEP.String()
}
