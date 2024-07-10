package step_executors

import "ai-developer/app/workflow_executors/step_executors/steps"

type PackageInstallStepExecutor interface {
	StepExecutor
	Execute(step steps.PackageInstallStep) error
}
