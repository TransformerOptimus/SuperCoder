package workflow_executors

import (
	"ai-developer/app/services"
	executors "ai-developer/app/workflow_executors/step_executors"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"errors"
	"fmt"
)

type WorkflowExecutor struct {
	executors            map[steps.StepName]executors.StepExecutor
	projectService       *services.ProjectService
	executionService     *services.ExecutionService
	executionStepService *services.ExecutionStepService
	activityLogService   *services.ActivityLogService
	storyService         *services.StoryService
}

func (we *WorkflowExecutor) Execute(workflowConfig *WorkflowConfig, args *WorkflowExecutionArgs) (err error) {

	fmt.Printf("Executing workflow for story ID: %d\n", args.StoryId)
	fmt.Println("Is Re-Execution: ", args.IsReExecution)
	fmt.Println("Branch: ", args.Branch)
	fmt.Println("Pull Request ID: ", args.PullRequestId)
	fmt.Println("Execution ID: ", args.ExecutionId)

	execution, err := we.executionService.GetExecutionByID(uint(args.ExecutionId))
	if err != nil {
		fmt.Printf("Error creating execution: %s\n", err.Error())
		return
	}
	fmt.Println("Execution created: ", execution)
	fmt.Println("Branch: ", execution.BranchName)

	story, err := we.storyService.GetStoryById(args.StoryId)
	if err != nil {
		fmt.Printf("Error fetching story: %s\n", err.Error())
		return
	}

	project, err := we.projectService.GetProjectById(story.ProjectID)

	workflowConfig.StepGraph.Walk(func(name steps.StepName, step steps.WorkflowStep) error {
		executor, ok := we.executors[name]
		if !ok {
			return errors.New("executor not found")
		}

		executionStep, err := we.executionStepService.CreateExecutionStep(
			execution.ID,
			step.StepName(),
			step.StepType(),
			nil,
		)

		if err != nil {
			return err
		}

		switch name {
		case steps.CODE_GENERATE_STEP:
			generateCodeStep := step.(*steps.GenerateCodeStep)
			generateCodeStep.WithStory(story)
			generateCodeStep.WithProject(project)
			generateCodeStep.WithExecution(execution)
			generateCodeStep.WithExecutionStep(executionStep)
			generateCodeStep.WithPullRequestID(uint(args.PullRequestId))
			return executor.(executors.CodeGenerationExecutor).Execute(*generateCodeStep)
		case steps.CODE_GENERATE_CSS_STEP:
			generateCodeStep := step.(*steps.GenerateCodeStep)
			generateCodeStep.WithStory(story)
			generateCodeStep.WithProject(project)
			generateCodeStep.WithExecution(execution)
			generateCodeStep.WithExecutionStep(executionStep)
			return executor.(executors.CodeGenerationExecutor).Execute(*generateCodeStep)
		case steps.CODE_GENERATE_PAGE_STEP:
			generateCodeStep := step.(*steps.GenerateCodeStep)
			generateCodeStep.WithStory(story)
			generateCodeStep.WithProject(project)
			generateCodeStep.WithExecution(execution)
			generateCodeStep.WithExecutionStep(executionStep)
			return executor.(executors.CodeGenerationExecutor).Execute(*generateCodeStep)
		case steps.CODE_GENERATE_LAYOUT_STEP:
			generateCodeStep := step.(*steps.GenerateCodeStep)
			generateCodeStep.WithStory(story)
			generateCodeStep.WithProject(project)
			generateCodeStep.WithExecution(execution)
			generateCodeStep.WithExecutionStep(executionStep)
			return executor.(executors.CodeGenerationExecutor).Execute(*generateCodeStep)
		case steps.RETRY_CODE_GENERATE_STEP:
			retryCodeGenerateStep := step.(*steps.GenerateCodeStep)
			retryCodeGenerateStep.WithStory(story)
			retryCodeGenerateStep.WithProject(project)
			retryCodeGenerateStep.WithExecution(execution)
			retryCodeGenerateStep.WithExecutionStep(executionStep)
			retryCodeGenerateStep.WithPullRequestID(uint(args.PullRequestId))
			return executor.(executors.CodeGenerationExecutor).Execute(*retryCodeGenerateStep)
		case steps.UPDATE_CODE_FILE_STEP:
			updateCodeFileStep := step.(*steps.UpdateCodeFileStep)
			updateCodeFileStep.WithStory(story)
			updateCodeFileStep.WithProject(project)
			updateCodeFileStep.WithExecution(execution)
			updateCodeFileStep.WithExecutionStep(executionStep)
			return executor.(executors.UpdateCodeFileExecutor).Execute(*updateCodeFileStep)
		case steps.UPDATE_CODE_CSS_FILE_STEP:
			updateCodeFileStep := step.(*steps.UpdateCodeFileStep)
			updateCodeFileStep.WithStory(story)
			updateCodeFileStep.WithProject(project)
			updateCodeFileStep.WithExecution(execution)
			updateCodeFileStep.WithExecutionStep(executionStep)
			return executor.(executors.UpdateCodeFileExecutor).Execute(*updateCodeFileStep)
		case steps.UPDATE_CODE_LAYOUT_FILE_STEP:
			updateCodeFileStep := step.(*steps.UpdateCodeFileStep)
			updateCodeFileStep.WithStory(story)
			updateCodeFileStep.WithProject(project)
			updateCodeFileStep.WithExecution(execution)
			updateCodeFileStep.WithExecutionStep(executionStep)
			return executor.(executors.UpdateCodeFileExecutor).Execute(*updateCodeFileStep)
		case steps.UPDATE_CODE_PAGE_FILE_STEP:
			updateCodeFileStep := step.(*steps.UpdateCodeFileStep)
			updateCodeFileStep.WithStory(story)
			updateCodeFileStep.WithProject(project)
			updateCodeFileStep.WithExecution(execution)
			updateCodeFileStep.WithExecutionStep(executionStep)
			return executor.(executors.UpdateCodeFileExecutor).Execute(*updateCodeFileStep)
		case steps.GIT_COMMIT_STEP:
			gitCommitStep := step.(*steps.GitCommitStep)
			gitCommitStep.WithStory(story)
			gitCommitStep.WithProject(project)
			gitCommitStep.WithExecution(execution)
			gitCommitStep.WithExecutionStep(executionStep)
			return executor.(executors.GitCommitExecutor).Execute(*gitCommitStep)
		case steps.GIT_CREATE_BRANCH_STEP:
			gitMakeBranchStep := step.(*steps.GitMakeBranchStep)
			gitMakeBranchStep.WithStory(story)
			gitMakeBranchStep.WithProject(project)
			gitMakeBranchStep.WithExecution(execution)
			gitMakeBranchStep.WithExecutionStep(executionStep)
			return executor.(executors.GitMakeBranchExecutor).Execute(*gitMakeBranchStep)
		case steps.GIT_PUSH_STEP:
			gitPushStep := step.(*steps.GitPushStep)
			gitPushStep.WithStory(story)
			gitPushStep.WithProject(project)
			gitPushStep.WithExecution(execution)
			gitPushStep.WithExecutionStep(executionStep)
			return executor.(executors.GitPushExecutor).Execute(*gitPushStep)
		case steps.GIT_CREATE_PULL_REQUEST_STEP:
			gitMakePullRequestStep := step.(*steps.GitMakePullRequestStep)
			gitMakePullRequestStep.WithStory(story)
			gitMakePullRequestStep.WithProject(project)
			gitMakePullRequestStep.WithExecution(execution)
			gitMakePullRequestStep.WithExecutionStep(executionStep)
			gitMakePullRequestStep.WithPullRequestID(uint(args.PullRequestId))
			return executor.(executors.GitMakePullRequestExecutor).Execute(*gitMakePullRequestStep)
		case steps.SERVER_START_STEP:
			serverStartTestStep := step.(*steps.ServerStartTestStep)
			serverStartTestStep.WithStory(story)
			serverStartTestStep.WithProject(project)
			serverStartTestStep.WithExecution(execution)
			serverStartTestStep.WithExecutionStep(executionStep)
			return executor.(executors.ServerStartTestExecutor).Execute(*serverStartTestStep)
		case steps.RESET_DB_STEP:
			resetDBStep := step.(*steps.ResetDBStep)
			resetDBStep.WithStory(story)
			resetDBStep.WithProject(project)
			resetDBStep.WithExecution(execution)
			resetDBStep.WithExecutionStep(executionStep)
			return executor.(executors.ResetDBStepExecutor).Execute(*resetDBStep)
		case steps.PACKAGE_INSTALL_STEP:
			resetDBStep := step.(*steps.PackageInstallStep)
			resetDBStep.WithStory(story)
			resetDBStep.WithProject(project)
			resetDBStep.WithExecution(execution)
			resetDBStep.WithExecutionStep(executionStep)
			return executor.(executors.PackageInstallStepExecutor).Execute(*resetDBStep)
		}

		return errors.New("step not found")
	})
	return nil
}

func NewWorkflowExecutor(
	executors map[steps.StepName]executors.StepExecutor,
	projectService *services.ProjectService,
	executionService *services.ExecutionService,
	executionStepService *services.ExecutionStepService,
	activityLogService *services.ActivityLogService,
	storyService *services.StoryService,
) *WorkflowExecutor {
	return &WorkflowExecutor{
		executors:            executors,
		projectService:       projectService,
		executionService:     executionService,
		executionStepService: executionStepService,
		activityLogService:   activityLogService,
		storyService:         storyService,
	}
}
