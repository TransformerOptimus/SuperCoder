package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/services/git_providers"
	"ai-developer/app/utils"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"fmt"
)

type GitPushExecutor struct {
	organisationService *services.OrganisationService
	gitnessService      *git_providers.GitnessService
	activeLogService    *services.ActivityLogService
}

func NewGitPushExecutor(
	organisationService *services.OrganisationService,
	gitnessService *git_providers.GitnessService,
	activeLogService *services.ActivityLogService,
) *GitPushExecutor {
	return &GitPushExecutor{
		organisationService: organisationService,
		gitnessService:      gitnessService,
		activeLogService:    activeLogService,
	}

}

func (e GitPushExecutor) Execute(step steps.GitPushStep) error {
	err := e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Pushing code changes to remote repository...")
	if err != nil {
		fmt.Printf("Error creating activity log: %s\n", err.Error())
		return err
	}
	organisation, err := e.organisationService.GetOrganisationByID(step.Project.OrganisationID)
	spaceOrProjectName := e.gitnessService.GetSpaceOrProjectName(organisation)
	origin := fmt.Sprintf("https://%s:%s@%s/git/%s/%s.git", config.GitnessUser(), config.GitnessToken(),
		config.GitnessHost(), spaceOrProjectName, step.Project.Name)
	branch := step.Execution.BranchName
	projectDir := config.WorkspaceWorkingDirectory() + "/" + step.Project.HashID
	err = utils.GitPush(projectDir, origin, branch)
	if err != nil {
		//TODO Handle Failure
		fmt.Printf("Error pushing to remote repository: %s\n", err.Error())
		return err
	}
	err = e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Code changes pushed successfully.")
	fmt.Println("Changes pushed successfully!")
	return nil
}
