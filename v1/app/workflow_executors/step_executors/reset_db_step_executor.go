package step_executors

import "ai-developer/app/workflow_executors/step_executors/steps"

type ResetDBStepExecutor interface {
	StepExecutor
	Execute(step steps.ResetDBStep) error
}
