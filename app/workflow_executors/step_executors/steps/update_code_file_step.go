package steps

type UpdateCodeFileStep struct {
	BaseStep
	WorkflowStep
	File  string
	Retry bool
	Type  string
}

func (s UpdateCodeFileStep) StepType() string {
	return FILE_OPERATION.String()
}

func (s UpdateCodeFileStep) StepName() string {
	return UPDATE_CODE_FILE_STEP.String()
}
