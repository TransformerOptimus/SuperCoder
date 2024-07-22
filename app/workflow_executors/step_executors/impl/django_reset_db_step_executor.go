package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/utils"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

type ResetDjangoDBStepExecutor struct {
	activeLogService *services.ActivityLogService
	logger           *zap.Logger
}

func NewResetDjangoDBStepExecutor(
	activeLogService *services.ActivityLogService,
	logger *zap.Logger,
) *ResetDjangoDBStepExecutor {
	return &ResetDjangoDBStepExecutor{
		activeLogService: activeLogService,
		logger:           logger.Named("ResetDjangoDBStepExecutor"),
	}
}

func (e ResetDjangoDBStepExecutor) Execute(step steps.ResetDBStep) error {
	e.logger.Info("Resetting Django DB...")

	err := e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Resetting Django DB...")
	if err != nil {
		e.logger.Error("Error creating activity log", zap.Error(err))
		return err
	}

	projectDir := config.WorkspaceWorkingDirectory() + "/" + step.Project.HashID

	// Remove the SQLite database file if it exists
	dbPath := filepath.Join(projectDir, "db.sqlite3")
	if err := utils.RemoveFile(dbPath); err != nil {
		e.logger.Error("Error removing database file", zap.Error(err))
		return err
	}

	migrationsFolderPath := filepath.Join(projectDir,"myapp", "migrations")
	// Remove migrations directories if they exist
	if err := e.removeMigrationFiles(migrationsFolderPath); err != nil {
		e.logger.Error("Error removing migrations directories", zap.Error(err))
		return err
	}

	// Ensure virtual environment is activated
	if err := e.setupVirtualEnv(projectDir); err != nil {
		e.logger.Error("Error activating virtual environment", zap.Error(err))
		return err
	}

	// Create new Django migrations
	if err := e.createDjangoMigrations(projectDir); err != nil {
		e.logger.Error("Error creating Django migrations", zap.Error(err))
		return err
	}

	// Apply Django migrations
	if err := e.applyDjangoMigrations(projectDir); err != nil {
		e.logger.Error("Error applying Django migrations", zap.Error(err))
		return err
	}

	err = e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Django DB reset successfully!")
	if err != nil {
		e.logger.Error("Error creating activity log", zap.Error(err))
		return err
	}

	e.logger.Info("Django DB reset successfully!")
	return nil
}

func (e ResetDjangoDBStepExecutor) removeMigrationFiles(folderPath string) error {
    return filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if info.IsDir() {
            // Skip the directory itself, but continue into subdirectories
            return nil
        }
        if filepath.Base(path) != "__init__.py" && strings.HasSuffix(path, ".py") {
            if err := os.Remove(path); err != nil {
                e.logger.Error("Error removing migration file", zap.String("file", path), zap.Error(err))
                return err
            }
            e.logger.Info("Removed migration file", zap.String("file", path))
        }
        
        return nil
    })
}

func (e ResetDjangoDBStepExecutor) setupVirtualEnv(projectDir string) error {
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

func (e ResetDjangoDBStepExecutor) createVirtualEnv(projectDir string) error {
	cmd := exec.Command("python3", "-m", "venv", ".venv")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error creating virtual environment: %w", err)
	}
	return nil
}

func (e ResetDjangoDBStepExecutor) createDjangoMigrations(projectDir string) error {
	pythonPath := filepath.Join(projectDir, ".venv", "bin", "python")
	err := utils.RunCommand(pythonPath, projectDir, "manage.py", "makemigrations")
	if err != nil {
		e.logger.Error("Error creating Django migrations", zap.Error(err))
		return err
	}
	return nil
}

func (e ResetDjangoDBStepExecutor) applyDjangoMigrations(projectDir string) error {
	pythonPath := filepath.Join(projectDir, ".venv", "bin", "python")
	err := utils.RunCommand(pythonPath, projectDir, "manage.py", "migrate")
	if err != nil {
		e.logger.Error("Error applying Django migrations", zap.Error(err))
		return err
	}
	return nil
}
