package steps

type GitPushStep struct {
	BaseStep
	WorkflowStep
	Type string
}

func (s GitPushStep) StepType() string {
	return COMMAND.String()
}

func (s GitPushStep) StepName() string {
	return GIT_PUSH_STEP.String()
}
