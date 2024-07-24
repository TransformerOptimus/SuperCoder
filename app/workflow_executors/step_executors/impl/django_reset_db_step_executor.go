package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/utils"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"os"
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