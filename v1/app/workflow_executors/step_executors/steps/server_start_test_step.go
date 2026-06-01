package steps

type ServerStartTestStep struct {
	BaseStep
	WorkflowStep
	Type string
}

func (s ServerStartTestStep) StepType() string {
	return CODE_TEST.String()
}

func (s ServerStartTestStep) StepName() string {
	return SERVER_START_STEP.String()
}
