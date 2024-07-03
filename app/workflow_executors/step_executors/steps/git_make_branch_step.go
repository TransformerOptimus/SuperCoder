package steps

type GitMakeBranchStep struct {
	BaseStep
	WorkflowStep
}

func (s GitMakeBranchStep) StepType() string {
	return GIT.String()
}

func (s GitMakeBranchStep) StepName() string {
	return GIT_CREATE_BRANCH_STEP.String()
}
