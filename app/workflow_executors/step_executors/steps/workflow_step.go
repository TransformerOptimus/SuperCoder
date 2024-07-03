package steps

type WorkflowStep interface {
	StepType() string
	StepName() string
}
