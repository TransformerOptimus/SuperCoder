package step_executors

import "ai-developer/app/workflow_executors/step_executors/steps"

type GitMakeBranchExecutor interface {
	StepExecutor
	Execute(step steps.GitMakeBranchStep) error
}
