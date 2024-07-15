package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/models"
	"ai-developer/app/services"
	"ai-developer/app/services/git_providers"
	"ai-developer/app/utils"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"fmt"
	"os"
	"path/filepath"
)

type GitMakeBranchExecutor struct {
	executionService    *services.ExecutionService
	activityLogService  *services.ActivityLogService
	gitnessService      *git_providers.GitnessService
	organisationService *services.OrganisationService
}

func NewGitMakeBranchExecutor(
	executionService *services.ExecutionService,
	activityLogService *services.ActivityLogService,
	organisationService *services.OrganisationService,
	gitnessService *git_providers.GitnessService,
) *GitMakeBranchExecutor {
	return &GitMakeBranchExecutor{
		executionService:    executionService,
		activityLogService:  activityLogService,
		organisationService: organisationService,
		gitnessService:      gitnessService,
	}

}

func (e GitMakeBranchExecutor) Execute(step steps.GitMakeBranchStep) error {
	//Handle git make branch step by calling required function
	fmt.Printf("Executing Step '%s' for Project '%s'...\n", step.StepName(), step.Project.Name)
	err := e.activityLogService.CreateActivityLog(step.ExecutionStep.ExecutionID, step.ExecutionStep.ID, "INFO", fmt.Sprintf("Setting up working directory, checking out feature branch and pulling latest code..."))
	if err != nil {
		return err
	}

	projectDir := config.WorkspaceWorkingDirectory() + "/" + step.Project.HashID
	workingDir := projectDir
	branchName := step.Execution.BranchName

	//STEP -
	if !step.Execution.ReExecution {
		if !e.isGitInitializedInRoot(workingDir) {
			err := e.initializeGitWithConfig(workingDir)
			if err != nil {
				fmt.Printf("Error initializing Git repository: %s\n", err)
				return err
			}
			fmt.Println("Initialized Git repository in the working directory, adding updating logs")
			err = e.activityLogService.CreateActivityLog(step.ExecutionStep.ExecutionID, step.ExecutionStep.ID, "INFO", "Initialized Git repository in the working directory.")
			if err != nil {
				fmt.Printf("Error creating activity log: %s\n", err.Error())
				return err
			}
		} else {
			err := e.configureGit(workingDir)
			if err != nil {
				fmt.Printf("Error configuring Git: %s\n", err)
				return err
			}
		}

		err = e.checkoutAndPullMain(workingDir, step.Project)
		if err != nil {
			fmt.Printf("Error checking out to main and pulling latest changes: %s\n", err)
			return err
		}
		fmt.Println("Creating new branch")
		err = utils.CreateBranch(workingDir, branchName)
		if err != nil {
			fmt.Printf("Error creating branch: %s\n", err)
			return err
		}
		err = e.activityLogService.CreateActivityLog(step.ExecutionStep.ExecutionID, step.ExecutionStep.ID, "INFO", fmt.Sprintf("Created new branch '%s'.", branchName))
		if err != nil {
			fmt.Printf("Error creating activity log: %s\n", err.Error())
			return err
		}
	} else {
		fmt.Printf("Re-execution flag is set. Attempting to switch to existing branch '%s'.\n", branchName)
		err := utils.CheckoutBranch(workingDir, branchName)
		if err != nil {
			fmt.Printf("Error checking out branch: %s\n", err)
			return err
		}

		err = e.activityLogService.CreateActivityLog(step.ExecutionStep.ExecutionID, step.ExecutionStep.ID, "INFO", fmt.Sprintf("Switched to branch '%s'.", branchName))
		if err != nil {
			fmt.Printf("Error creating activity log: %s\n", err.Error())
			return err
		}
	}

	err = e.activityLogService.CreateActivityLog(step.ExecutionStep.ExecutionID, step.ExecutionStep.ID, "INFO", fmt.Sprintf("Checked out to latest branch and pulled latest code!"))
	if err != nil {
		fmt.Printf("Error creating activity log: %s\n", err.Error())
		return err
	}
	return nil
}

func (e *GitMakeBranchExecutor) isGitInitializedInRoot(workingDir string) bool {
	fmt.Printf("Checking if Git is initialized in the working directory: %s\n", workingDir)
	gitDir := filepath.Join(workingDir, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return false
	}
	return true
}

func (e *GitMakeBranchExecutor) initializeGitWithConfig(workingDir string) error {
	fmt.Printf("Initializing Git repository in the working directory: %s\n", workingDir)
	_, err := utils.InitialiseGit(workingDir)
	if err != nil {
		fmt.Printf("Error initializing Git repository: %s\n", err)
		return err
	}
	err = e.configureGit(workingDir)
	if err != nil {
		fmt.Printf("Error configuring Git: %s\n", err)
		return err
	}
	return nil
}

func (e *GitMakeBranchExecutor) configureGit(workingDir string) error {
	// Configure the directory as a safe directory for Git operations
	err := utils.ConfigGitSafeDir(config.WorkspaceWorkingDirectory())
	if err != nil {
		fmt.Printf("Error configuring safe directory: %s\n", err)
		return err
	}
	// Set global configuration for user email
	err = utils.ConfigGitUserEmail(config.WorkspaceWorkingDirectory())
	if err != nil {
		fmt.Printf("Error configuring global user email: %s\n", err)
		return err
	}
	// Set global configuration for username
	err = utils.ConfigureGitUserName(config.WorkspaceWorkingDirectory())
	if err != nil {
		fmt.Printf("Error configuring global user name: %s\n", err)
		return err
	}
	// Set global configuration for pull rebase
	err = utils.ConfigGitPullRebaseTrue(workingDir)
	if err != nil {
		fmt.Printf("Error configuring pull rebase: %s\n", err)
	}
	return nil
}

func (e *GitMakeBranchExecutor) checkoutAndPullMain(workingDir string, project *models.Project) error {
	fmt.Printf("Checking out to main, pulling latest changes\n")
	//Checkout to main
	err := utils.CheckoutBranch(workingDir, "main")
	if err != nil {
		return err
	}
	organisation, err := e.organisationService.GetOrganisationByID(uint(int(project.OrganisationID)))
	GitnessSpaceOrProjectName := e.gitnessService.GetSpaceOrProjectName(organisation)
	err = utils.PullOriginBranch(workingDir, project, GitnessSpaceOrProjectName)
	if err != nil {
		fmt.Println("Error pulling latest changes: ", err)
		return err
	}

	return nil
}
