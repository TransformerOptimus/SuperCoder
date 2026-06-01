package step_executors

import "ai-developer/app/workflow_executors/step_executors/steps"

type GitPushExecutor interface {
	StepExecutor
	Execute(step steps.GitPushStep) error
}
