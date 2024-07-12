package impl

import (
	"ai-developer/app/services"
	"ai-developer/app/services/git_providers"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"fmt"
	"strconv"
)

type GitnessMakePullRequestExecutor struct {
	storyService           *services.StoryService
	executionService       *services.ExecutionService
	executionOutputService *services.ExecutionOutputService
	organisationService    *services.OrganisationService
	gitnessService         *git_providers.GitnessService
	pullRequestService     *services.PullRequestService
	activeLogService       *services.ActivityLogService
}

func NewGitnessMakePullRequestExecutor(
	storyService *services.StoryService,
	executionService *services.ExecutionService,
	organisationService *services.OrganisationService,
	executionOutputService *services.ExecutionOutputService,
	gitnessService *git_providers.GitnessService,
	pullRequestService *services.PullRequestService,
	activeLogService *services.ActivityLogService,
) *GitnessMakePullRequestExecutor {
	return &GitnessMakePullRequestExecutor{
		storyService:           storyService,
		executionService:       executionService,
		organisationService:    organisationService,
		executionOutputService: executionOutputService,
		gitnessService:         gitnessService,
		pullRequestService:     pullRequestService,
		activeLogService:       activeLogService,
	}
}

func (e GitnessMakePullRequestExecutor) Execute(step steps.GitMakePullRequestStep) error {
	fmt.Printf("Executing Step '%s' for Project '%s'...\n", step.StepName(), step.Project.Name)
	fmt.Println("RE-EXECUTION : ", step.Execution.ReExecution)
	organisation, err := e.organisationService.GetOrganisationByID(step.Project.OrganisationID)
	spaceOrProjectName := e.gitnessService.GetSpaceOrProjectName(organisation)
	if !step.Execution.ReExecution {
		pr, err := e.gitnessService.CreatePullRequest(spaceOrProjectName, step.Project.Name, step.Execution.BranchName, "main",
			"Pull Request: "+step.Story.Title, "Auto-generated pull request")
		if err != nil {
			fmt.Printf("Error creating pull request: %s\n", err.Error())
			return err
		}

		err = e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", fmt.Sprintf("Pull request created successfully. %s", pr.Title))
		if err != nil {
			fmt.Printf("Error creating activity log: %s\n", err.Error())
			return err
		}

		// Pass PR details to EndExecution via options
		optionsMap := map[string]interface{}{
			"pr_name":          pr.Title,
			"pr_number":        pr.Number,
			"pr_description":   pr.Description,
			"source_sha":       pr.SourceSHA,
			"merge_target_sha": pr.MergeTargetSHA,
			"merge_base_sha":   pr.MergeBaseSHA,
			"story_id":         step.Execution.StoryID,
		}

		err = e.handleExecutionOutput(step.ExecutionStep.ExecutionID, optionsMap)
		if err != nil {
			fmt.Printf("Error handling execution: %s\n", err.Error())
			return err
		}
		err = e.activeLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Pull request created successfully.")
		if err != nil {
			fmt.Printf("Error creating activity log: %s\n", err.Error())
			return err
		}
	} else {
		// Rerun PR with comments
		pullRequest, err := e.pullRequestService.GetPullRequestByID(step.PullRequestID)
		if err != nil {
			fmt.Printf("Error getting pull request by execution output: %s\n", err.Error())
			return err
		}
		newPullRequestData, err := e.gitnessService.FetchPullRequest(spaceOrProjectName, step.Project.Name, pullRequest.PullRequestNumber)
		if err != nil {
			fmt.Printf("Error fetching pull request data: %s\n", err.Error())
			return err
		}
		err = e.pullRequestService.UpdatePullRequestSourceSHA(pullRequest, newPullRequestData.SourceSHA)
		if err != nil {
			fmt.Printf("Error updating pull request source SHA: %s\n", err.Error())
			return err
		}
		err = e.handleExecutionOutput(step.ExecutionStep.ExecutionID)
		if err != nil {
			fmt.Printf("Error handling execution: %s\n", err.Error())
			return err
		}
	}

	//Update Execution Step Status
	err = e.executionService.UpdateExecutionStatus(step.ExecutionStep.ExecutionID, "DONE")
	if err != nil {
		fmt.Printf("Error updating execution status: %s\n", err.Error())
		return err
	}
	fmt.Println("Execution Step Status Updated to DONE")
	//Update Story Status
	err = e.storyService.UpdateStoryStatus(int(step.Execution.StoryID), "DONE")
	if err != nil {
		fmt.Printf("Error updating story status: %s\n", err.Error())
		return err
	}
	fmt.Println("Story Status Updated to DONE")
	return nil
}

func (e *GitnessMakePullRequestExecutor) handleExecutionOutput(executionID uint, options ...map[string]interface{}) error {
	fmt.Printf("Ending Git Make Pull Request Step for Execution ID: %d\n", executionID)
	fmt.Printf("Options: %v\n", options)
	// Extract PR details from options
	if len(options) > 0 {
		fmt.Printf("Options: %v\n", options)
		prDetails := options[0]
		fmt.Println("PR Details: ", prDetails)
		prName := prDetails["pr_name"].(string)
		prNumber := prDetails["pr_number"].(int)
		prDescription := prDetails["pr_description"].(string)
		sourceSHA := prDetails["source_sha"].(string)
		//mergeTargetSHA := prDetails["merge_target_sha"].(string)
		mergeTargetSHA := "sample"
		mergeBaseSHA := prDetails["merge_base_sha"].(string)
		storyID := prDetails["story_id"].(uint)
		fmt.Printf("PR Details: %s, %d, %s, %s, %s, %s\n", prName, prNumber, prDescription, sourceSHA, mergeTargetSHA, mergeBaseSHA)
		executionOutput, err := e.executionOutputService.CreateExecutionOutput(executionID)
		if err != nil {
			return err
		}
		prType := "automated"
		pullRequest, err2 := e.pullRequestService.CreatePullRequest(prName, prDescription, strconv.Itoa(prNumber), "GITNESS",
			sourceSHA, mergeTargetSHA, mergeBaseSHA, prNumber, storyID, executionOutput.ID, prType)
		if err2 != nil {
			fmt.Printf("Error creating execution output: %s\n", err2.Error())
			return err2
		}
		fmt.Printf("Execution output created successfully: %v\n", pullRequest)
	}
	return nil
}
