package step_executors

import "ai-developer/app/workflow_executors/step_executors/steps"

type GitMakePullRequestExecutor interface {
	StepExecutor
	Execute(step steps.GitMakePullRequestStep) error
}
