package step_executors

import "ai-developer/app/workflow_executors/step_executors/steps"

type UpdateCodeFileExecutor interface {
	StepExecutor
	Execute(step steps.UpdateCodeFileStep) error
}
