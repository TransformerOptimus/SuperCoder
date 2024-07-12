package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/utils"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"fmt"
	"strings"
)

type GitCommitExecutor struct {
	executionService *services.ExecutionService
	activeLogService *services.ActivityLogService
}

// TODO: Move util out of stuct
func NewGitCommitExecutor(
	executionService *services.ExecutionService,
	activityLogService *services.ActivityLogService,

) *GitCommitExecutor {
	return &GitCommitExecutor{
		executionService: executionService,
		activeLogService: activityLogService,
	}
}

func (e GitCommitExecutor) Execute(step steps.GitCommitStep) error {
	err := e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Committing code changes...")
	if err != nil {
		fmt.Println("Error creating activity log" + err.Error())
		return err
	}
	workingDir := config.WorkspaceWorkingDirectory() + "/" + step.Project.HashID

	currentBranch, err := utils.GetCurrentBranch(workingDir)
	if err != nil {
		fmt.Printf("Error getting current branch: %s\n", err.Error())
		return err
	}

	if currentBranch != step.Execution.BranchName {
		fmt.Printf("Current branch '%s' does not match execution branch '%s'\n", currentBranch, step.Execution.BranchName)
		return fmt.Errorf("current branch '%s' does not match execution branch '%s'", currentBranch, step.Execution.BranchName)
	}
	commitMessage := "Update Code for " + step.Story.Title
	commitID, err := e.makeCommit(workingDir, commitMessage)
	if err != nil {
		fmt.Printf("Error making commit: %s\n", err.Error())
		return err
	}
	err = e.executionService.UpdateCommitID(step.Execution, commitID)
	if err != nil {
		fmt.Printf("Error updating execution with commit ID: %s\n", err.Error())
		return err
	}

	err = e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Code changes committed successfully.")
	if err != nil {
		fmt.Println("Error creating activity log" + err.Error())
		return err
	}
	return nil
}

func (e *GitCommitExecutor) makeCommit(workingDir, commitMessage string) (string, error) {

	// Set global configuration for user email and name
	err := utils.ConfigGitUserEmail(workingDir)
	if err != nil {
		fmt.Printf("Error setting global git user Email")
		return "", err
	}
	err = utils.ConfigureGitUserName(workingDir)
	if err != nil {
		fmt.Printf("Error setting global git user name")
		return "", err

	}
	_, err = utils.GitAddToTrackFiles(workingDir, err)
	if err != nil {
		fmt.Printf("Error adding files to track: %s\n", err)
		return "", err
	}
	output, err := utils.GitCommitWithMessage(workingDir, commitMessage, err)
	if err != nil {
		fmt.Printf("Error committing changes: %s\n", output)
		return "", err
	}
	fmt.Printf("Commit output: %s\n", output)
	commitIDOutput, err := utils.GetLatestCommitID(workingDir, err)
	if err != nil {
		fmt.Printf("Error getting latest commit ID: %s\n", err)
		return "", err
	}
	return strings.TrimSpace(commitIDOutput), nil
}
