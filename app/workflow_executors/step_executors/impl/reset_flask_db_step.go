package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/utils"
	"fmt"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"path/filepath"

	"ai-developer/app/services"
	"ai-developer/app/workflow_executors/step_executors/steps"
)

type ResetFlaskDBStepExecutor struct {
	activeLogService *services.ActivityLogService
	logger           *zap.Logger
}

func NewResetFlaskDBStepExecutor(
	activeLogService *services.ActivityLogService,
	logger *zap.Logger,
) *ResetFlaskDBStepExecutor {
	return &ResetFlaskDBStepExecutor{
		activeLogService: activeLogService,
		logger:           logger.Named("ResetFlaskDBStepExecutor"),
	}
}

func (e ResetFlaskDBStepExecutor) Execute(step steps.ResetDBStep) error {
	e.logger.Info("Resetting Flask DB...")

	err := e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Resetting Flask DB...")
	if err != nil {
		e.logger.Error("Error creating activity log", zap.Error(err))
		return err
	}

	projectDir := config.WorkspaceWorkingDirectory() + "/" + step.Project.HashID

	// Remove instance and migrations directories if they exist
	if err := e.removeDir(projectDir + "/instance"); err != nil {
		e.logger.Error("Error removing instance directory", zap.Error(err))
		return err
	}
	if err := e.removeDir(projectDir + "/migrations"); err != nil {
		e.logger.Error("Error removing migrations directory", zap.Error(err))
		return err
	}

	// Ensure virtual environment is activated
	if err := e.setupVirtualEnv(projectDir); err != nil {
		e.logger.Error("Error activating virtual environment", zap.Error(err))
		return err
	}

	// Check environment before initializing the Flask DB
	if err := e.checkEnvironment(projectDir); err != nil {
		e.logger.Error("Environment check failed", zap.Error(err))
		return err
	}

	// Initialize the Flask DB
	if err := e.initFlaskDB(projectDir); err != nil {
		e.logger.Error("Error initializing Flask DB", zap.Error(err))
		return err
	}

	// Migrate the Flask DB
	if err := e.migrateFlaskDB(projectDir); err != nil {
		e.logger.Error("Error migrating Flask DB", zap.Error(err))
		return err
	}

	// Upgrade the Flask DB
	if err := e.upgradeFlaskDB(projectDir); err != nil {
		e.logger.Error("Error upgrading Flask DB", zap.Error(err))
		return err
	}

	err = e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Flask DB reset successfully.")
	if err != nil {
		e.logger.Error("Error creating activity log", zap.Error(err))
		return err
	}

	e.logger.Info("Flask DB reset successfully!")
	return nil
}

func (e ResetFlaskDBStepExecutor) removeDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		e.logger.Info("Directory does not exist", zap.String("dir", dir))
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		e.logger.Error("Error removing directory", zap.String("dir", dir), zap.Error(err))
		return err
	}
	return nil
}

func (e ResetFlaskDBStepExecutor) initFlaskDB(projectDir string) error {
	pythonPath := filepath.Join(projectDir, ".venv", "bin", "python")
	if err := utils.RunCommand(pythonPath, projectDir, "-m", "flask", "db", "init"); err != nil {
		e.logger.Error("Error initializing Flask DB", zap.Error(err))
		return err
	}
	return nil
}

func (e ResetFlaskDBStepExecutor) migrateFlaskDB(projectDir string) error {
	pythonPath := filepath.Join(projectDir, ".venv", "bin", "python")
	err := utils.RunCommand(pythonPath, projectDir, "-m", "flask", "db", "migrate", "-m", "latest_migration")
	if err != nil {
		e.logger.Error("Error running Flask DB migrate", zap.Error(err))
		return err
	}
	return nil
}

func (e ResetFlaskDBStepExecutor) upgradeFlaskDB(projectDir string) error {
	pythonPath := filepath.Join(projectDir, ".venv", "bin", "python")
	if err := utils.RunCommand(pythonPath, projectDir, "-m", "flask", "db", "upgrade"); err != nil {
		e.logger.Error("Error running Flask DB upgrade", zap.Error(err))
		return err
	}
	return nil
}

func (e ResetFlaskDBStepExecutor) setupVirtualEnv(projectDir string) error {
	venvPath := filepath.Join(projectDir, ".venv")
	venvBin := filepath.Join(venvPath, "bin")

	if _, err := os.Stat(venvPath); os.IsNotExist(err) {
		if err := e.createVirtualEnv(projectDir); err != nil {
			e.logger.Error("Error creating virtual environment", zap.Error(err))
			return err
		}
	}

	newPath := fmt.Sprintf("PATH=%s:%s", venvBin, os.Getenv("PATH"))
	if err := os.Setenv("PATH", newPath); err != nil {
		e.logger.Error("Error setting PATH environment variable", zap.Error(err))
		return err
	}
	return nil
}

func (e ResetFlaskDBStepExecutor) createVirtualEnv(projectDir string) error {
	cmd := exec.Command("python3", "-m", "venv", ".venv")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error creating virtual environment: %w", err)
	}
	return nil
}

func (e ResetFlaskDBStepExecutor) checkEnvironment(projectDir string) error {
	e.logger.Info("Checking environment...")
	e.logger.Info("PATH", zap.String("path", os.Getenv("PATH")))

	pythonPath := filepath.Join(projectDir, ".venv", "bin", "python")
	if _, err := exec.LookPath(pythonPath); err != nil {
		e.logger.Error("Python command not found in PATH", zap.Error(err))
		e.listInstalledPackages(projectDir)
		return err
	}

	e.logger.Info("Python command found in PATH")
	return nil
}

func (e ResetFlaskDBStepExecutor) listInstalledPackages(projectDir string) {
	venvPath := filepath.Join(projectDir, ".venv")
	venvBin := filepath.Join(venvPath, "bin")
	pipPath := filepath.Join(venvBin, "pip")

	cmd := exec.Command(pipPath, "list")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		e.logger.Error("Error listing installed packages", zap.Error(err))
		return
	}
}
