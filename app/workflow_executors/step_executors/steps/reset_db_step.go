package steps

type ResetDBStep struct {
	BaseStep
	WorkflowStep
	Type string
}

func (s ResetDBStep) StepType() string {
	return COMMAND.String()
}

func (s ResetDBStep) StepName() string {
	return RESET_DB_STEP.String()
}
