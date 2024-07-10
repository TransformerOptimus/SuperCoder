package steps

type PackageInstallStep struct {
	BaseStep
	WorkflowStep
	Type string
}

func (s PackageInstallStep) StepType() string {
	return COMMAND.String()
}

func (s PackageInstallStep) StepName() string {
	return PACKAGE_INSTALL_STEP.String()
}
