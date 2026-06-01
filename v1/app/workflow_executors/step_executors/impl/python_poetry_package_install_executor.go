package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"go.uber.org/zap"
	"os/exec"
)

type PackageInstallStepExecutor struct {
	activeLogService *services.ActivityLogService
	logger           *zap.Logger
}

func NewPackageInstallStepExecutor(
	activeLogService *services.ActivityLogService,
	logger *zap.Logger,
) *PackageInstallStepExecutor {
	return &PackageInstallStepExecutor{
		activeLogService: activeLogService,
		logger:           logger.Named("PackageInstallStepExecutor"),
	}
}

func (e PackageInstallStepExecutor) Execute(step steps.PackageInstallStep) error {
	e.logger.Info("Installing Poetry Packages ...")

	err := e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Installing Poetry Packages ...")
	if err != nil {
		e.logger.Error("Error creating activity log", zap.Error(err))
		return err
	}

	projectDir := config.WorkspaceWorkingDirectory() + "/" + step.Project.HashID

	err = e.PoetryInstall(projectDir, err)
	if err != nil {
		e.logger.Error("Error installing poetry", zap.Error(err))
		return err
	}

	err = e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Poetry Packages Installed Successfully!")
	if err != nil {
		e.logger.Error("Error creating activity log", zap.Error(err))
		return err
	}

	e.logger.Info("Poetry Packages Installed Successfully!")
	return nil
}

func (e PackageInstallStepExecutor) PoetryInstall(workDir string, err error) error {
	cmd := exec.Command("poetry", "install")
	cmd.Dir = workDir
	err = cmd.Run()
	return err
}
