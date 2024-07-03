package step_executors

import "ai-developer/app/workflow_executors/step_executors/steps"

type GitCommitExecutor interface {
	StepExecutor
	Execute(step steps.GitCommitStep) error
}
