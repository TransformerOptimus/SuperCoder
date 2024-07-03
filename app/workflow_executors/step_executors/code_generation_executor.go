package step_executors

import "ai-developer/app/workflow_executors/step_executors/steps"

type CodeGenerationExecutor interface {
	StepExecutor
	Execute(step steps.GenerateCodeStep) error
}
