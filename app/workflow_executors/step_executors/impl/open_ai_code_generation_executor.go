package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/constants"
	"ai-developer/app/llms"
	"ai-developer/app/models"
	"ai-developer/app/monitoring"
	"ai-developer/app/services"
	"ai-developer/app/utils"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type OpenAICodeGenerator struct {
	projectService            *services.ProjectService
	executionStepService      *services.ExecutionStepService
	executionService          *services.ExecutionService
	storyService              *services.StoryService
	pullRequestCommentService *services.PullRequestCommentsService
	activityLogService        *services.ActivityLogService
	llmAPIKeyService          *services.LLMAPIKeyService
	slackAlert                *monitoring.SlackAlert
}

func NewOpenAICodeGenerator(
	projectService *services.ProjectService,
	executionStepService *services.ExecutionStepService,
	executionService *services.ExecutionService,
	storyService *services.StoryService,
	pullRequestCommentService *services.PullRequestCommentsService,
	activityLogService *services.ActivityLogService,
	llmAPIKeyService *services.LLMAPIKeyService,
	slackAlert *monitoring.SlackAlert,
) *OpenAICodeGenerator {
	return &OpenAICodeGenerator{
		projectService:            projectService,
		executionStepService:      executionStepService,
		executionService:          executionService,
		storyService:              storyService,
		pullRequestCommentService: pullRequestCommentService,
		activityLogService:        activityLogService,
		llmAPIKeyService:          llmAPIKeyService,
		slackAlert:                slackAlert,
	}

}

func (openAICodeGenerator OpenAICodeGenerator) Execute(step steps.GenerateCodeStep) error {
	fmt.Printf("Executing GenerateCodeStep: %s\n", step.StepName())
	fmt.Printf("Working on project details: %v\n", step.Project)
	fmt.Printf("Working on pull request ID: %d\n", step.PullRequestID)
	fmt.Printf("Max loop iterations: %d\n", step.MaxLoopIterations)
	fmt.Printf("Is re-execution: %v\n", step.Execution.ReExecution)
	err := openAICodeGenerator.activityLogService.CreateActivityLog(
		step.Execution.ID,
		step.ExecutionStep.ID,
		"INFO",
		fmt.Sprintf("Code generation has started ..."),
	)
	if err != nil {
		fmt.Printf("Error creating activity log: %s\n", err.Error())
		return err
	}
	projectDir := config.WorkspaceWorkingDirectory() + "/" + step.Project.HashID
	fmt.Println("____________Project Directory: ", projectDir)
	fmt.Println("___________Checking for Max Retry______________")
	count, err := openAICodeGenerator.executionStepService.CountExecutionStepsOfName(step.Execution.ID, steps.CODE_GENERATE_STEP.String())
	if err != nil {
		fmt.Printf("Error checking max retry for generation: %s\n", err.Error())
		return err
	}
	fmt.Printf("Count of LLM steps: %d\n", count)
	if count > step.MaxLoopIterations {
		fmt.Println("Max retry limit reached for LLM steps")
		//Update story status to MAX_LOOP_ITERATION_REACHED
		if err := openAICodeGenerator.storyService.UpdateStoryStatus(int(step.Story.ID), "MAX_LOOP_ITERATION_REACHED"); err != nil {
			fmt.Printf("Error updating story status: %s\n", err.Error())
			return err
		}
		//Update execution status to MAX_LOOP_ITERATION_REACHED
		if err := openAICodeGenerator.executionService.UpdateExecutionStatus(step.Execution.ID, "MAX_LOOP_ITERATION_REACHED"); err != nil {
			fmt.Printf("Error updating execution step: %s\n", err.Error())
			return err
		}
		//Add all code to stage
		output, err := utils.GitAddToTrackFiles(projectDir, nil)
		if err != nil {
			fmt.Printf("Error adding files to track: %s\n", err.Error())
			return err
		}
		fmt.Printf("Git add output: %s\n", output)

		//Handle workspace clean up by commiting could be stashing or other ways later
		output, err = utils.GitCommitWithMessage(
			projectDir,
			"Max retry limit reached for code generation, committing code!",
			nil,
		)
		fmt.Printf("Git commit output: %s\n", output)
		if err != nil {
			fmt.Printf("Error commiting code: %s\n", err.Error())
			return err
		}

		err = openAICodeGenerator.activityLogService.CreateActivityLog(
			step.Execution.ID,
			step.ExecutionStep.ID,
			"ERROR",
			"Max retry limit reached for code generation!",
		)
		if err != nil {
			fmt.Printf("Error creating activity log: %s\n", err.Error())
			return err
		}

		err = openAICodeGenerator.slackAlert.SendAlert(
			"Max retry limit reached for code generation!",
			map[string]string{
				"story_id":          fmt.Sprintf("%d", int64(step.Story.ID)),
				"execution_id":      fmt.Sprintf("%d", int64(step.Execution.ID)),
				"execution_step_id": fmt.Sprintf("%d", int64(step.ExecutionStep.ID)),
				"pull_request_id":   fmt.Sprintf("%d", int64(step.PullRequestID)),
				"is_re_execution":   fmt.Sprintf("%t", step.Execution.ReExecution),
			})
		if err != nil {
			fmt.Printf("Error sending slack alert: %s\n", err.Error())
			return err
		}

		return fmt.Errorf("max retry limit reached for LLM steps")
	}

	finalInstructionForGeneration, err := openAICodeGenerator.buildFinalInstructionForGeneration(step)
	if err != nil {
		fmt.Printf("Error building final instruction for generation: %s\n", err.Error())
		return err
	}
	fmt.Printf("_________Final Instruction for Generation__________: %s\n", finalInstructionForGeneration)

	//extracting api_key from executionId
	story, err := openAICodeGenerator.storyService.GetStoryByExecutionID(step.Execution.ID)
	if err != nil {
		fmt.Println("Error getting story by execution ID: ", err)
	}
	projectId := story.ProjectID
	project, err := openAICodeGenerator.projectService.GetProjectById(projectId)
	if err != nil {
		fmt.Println("Error getting project by ID: ", err)
	}
	organisationId := project.OrganisationID
	llmAPIKey, err := openAICodeGenerator.llmAPIKeyService.GetLLMAPIKeyByModelName(constants.GPT_4O, uint(organisationId))
	if err != nil {
		fmt.Println("Error getting openai api key: ", err)
	}
	if llmAPIKey == nil || llmAPIKey.LLMAPIKey == "" {
		fmt.Println("______API Key not found in database_____")
		settingsUrl := config.Get("app.url").(string) + "/settings"
		err := openAICodeGenerator.activityLogService.CreateActivityLog(
			step.Execution.ID,
			step.ExecutionStep.ID,
			"INFO",
			fmt.Sprintf("Action required: There's an issue with your LLM API Key. Ensure your API Key for %s is correct. <a href='%s' style='color:%s; text-decoration:%s;'>Settings</a>", constants.GPT_4O, settingsUrl, "blue", "underline"),
		)
		if err != nil {
			fmt.Printf("Error creating activity log: %s\n", err.Error())
			return err
		}
		//Update Execution Status and Story Status
		if err := openAICodeGenerator.storyService.UpdateStoryStatus(int(step.Story.ID), constants.InReviewLLMKeyNotFound); err != nil {
			fmt.Printf("Error updating story status: %s\n", err.Error())
			return err
		}
		//Update execution status to MAX_LOOP_ITERATION_REACHED
		if err := openAICodeGenerator.executionService.UpdateExecutionStatus(step.Execution.ID, constants.InReviewLLMKeyNotFound); err != nil {
			fmt.Printf("Error updating execution step: %s\n", err.Error())
			return err
		}
		//Add all code to stage
		output, err := utils.GitAddToTrackFiles(projectDir, nil)
		if err != nil {
			fmt.Printf("Error adding files to track: %s\n", err.Error())
			return err
		}
		fmt.Printf("Git add output: %s\n", output)
		//Handle workspace clean up by commiting could be stashing or other ways later
		output, err = utils.GitCommitWithMessage(
			projectDir,
			"llm api key error, committing code!",
			nil,
		)
		fmt.Printf("Git commit output: %s\n", output)
		if err != nil {
			fmt.Printf("Error commiting code: %s\n", err.Error())
			return err
		}
		errorString := fmt.Sprintf("LLM API Key for model %s not found in database", constants.GPT_4O)
		return errors.New(errorString)
	}
	apiKey := llmAPIKey.LLMAPIKey
	fmt.Println("_________API_KEY_________", apiKey)

	framework := project.BackendFramework
	fmt.Println("_________FRAMEWORK_________", framework)
	// Generate code using the final instruction
	code, err := openAICodeGenerator.GenerateCode(apiKey, framework, finalInstructionForGeneration, step.ExecutionStep, projectDir, step)
	if err != nil {
		fmt.Printf("Error generating code: %s\n", err.Error())
		return err
	}
	fmt.Printf("_________Generated Code__________: %s\n", code)

	// Update execution step with response
	if err := openAICodeGenerator.executionStepService.UpdateExecutionStepResponse(
		step.ExecutionStep,
		map[string]interface{}{
			"llm_response": code,
		},
		"SUCCESS"); err != nil {
		fmt.Printf("Error updating execution step: %s\n", err.Error())
		return err
	}

	err = openAICodeGenerator.activityLogService.CreateActivityLog(
		step.Execution.ID,
		step.ExecutionStep.ID,
		"INFO",
		"Code generation has completed successfully.",
	)
	if err != nil {
		fmt.Printf("Error creating activity log: %s\n", err.Error())
		return err
	}

	return nil
}

// GenerateCode uses OpenAI API to generate code based on the instruction.
func (openAICodeGenerator *OpenAICodeGenerator) GenerateCode(apiKey string, framework string, instruction string, executionStep *models.ExecutionStep, projectDir string, step steps.GenerateCodeStep) (string, error) {
	messages := openAICodeGenerator.generateMessages(framework, instruction, executionStep.ExecutionID, projectDir)
	err := openAICodeGenerator.executionStepService.UpdateExecutionStepRequest(
		executionStep,
		map[string]interface{}{
			"final_instruction": instruction,
			"llm_request":       messages,
		},
		"IN_PROGRESS",
	)
	openAIClient := llms.NewOpenAiClient(apiKey)
	response, err := openAIClient.ChatCompletion(messages)
	if err != nil {
		settingsUrl := config.Get("app.url").(string) + "/settings"
		err := openAICodeGenerator.activityLogService.CreateActivityLog(
			step.Execution.ID,
			step.ExecutionStep.ID,
			"INFO",
			fmt.Sprintf("Action required: There's an issue with your LLM API Key. Ensure your API Key for %s is correct. <a href='%s' style='color:%s; text-decoration:%s;'>Settings</a>", constants.GPT_4O, settingsUrl, "blue", "underline"),
		)
		if err != nil {
			fmt.Printf("Error creating activity log: %s\n", err.Error())
			return "", err
		}
		//Update Execution Status and Story Status
		if err := openAICodeGenerator.storyService.UpdateStoryStatus(int(step.Story.ID), constants.InReviewLLMKeyNotFound); err != nil {
			fmt.Printf("Error updating story status: %s\n", err.Error())
			return "", err
		}
		//Update execution status to MAX_LOOP_ITERATION_REACHED
		if err := openAICodeGenerator.executionService.UpdateExecutionStatus(step.Execution.ID, constants.InReviewLLMKeyNotFound); err != nil {
			fmt.Printf("Error updating execution step: %s\n", err.Error())
			return "", err
		}
		//Add all code to stage
		output, err := utils.GitAddToTrackFiles(projectDir, nil)
		if err != nil {
			fmt.Printf("Error adding files to track: %s\n", err.Error())
			return "", err
		}
		fmt.Printf("Git add output: %s\n", output)
		//Handle workspace clean up by commiting could be stashing or other ways later
		output, err = utils.GitCommitWithMessage(
			projectDir,
			"llm api key error, committing code!",
			nil,
		)
		fmt.Printf("Git commit output: %s\n", output)
		if err != nil {
			fmt.Printf("Error commiting code: %s\n", err.Error())
			return "", err
		}
		return "", fmt.Errorf("failed to generate code from OpenAI API: %w", err)
	}
	return response, nil
}

func (openAICodeGenerator *OpenAICodeGenerator) generateMessages(framework string, instruction string, executionId uint, projectDir string) []llms.OpenAiChatCompletionMessage {
	inputContext, err := openAICodeGenerator.createInputContext(projectDir)
	if err != nil {
		fmt.Printf("Failed to create input context: %v\n", err)
	}
	messages := []llms.OpenAiChatCompletionMessage{
		{Role: "system", Content: openAICodeGenerator.getSystemPrompt(framework, projectDir)},
		{Role: "user", Content: "The current codebase is:\n" + inputContext},
		{Role: "user", Content: instruction},
	}

	// Fetch the last execution step
	previousExecutionSteps, err := openAICodeGenerator.executionStepService.FetchExecutionSteps(
		executionId,
		steps.CODE_GENERATE_STEP.String(),
		steps.LLM.String(),
		1,
	)
	fmt.Println("__________STEPS : ________ ", previousExecutionSteps)
	if err == nil && len(previousExecutionSteps) > 0 {
		lastExecutionStep := previousExecutionSteps[0]
		fmt.Println("__________LAST EXECUTION STEP : ________ ", lastExecutionStep)
		if lastInput, ok := lastExecutionStep.Request["final_instruction"].(string); ok {
			fmt.Println("__________LAST INPUT : ________ ", lastInput)
			messages = append(messages, llms.OpenAiChatCompletionMessage{Role: "user", Content: "last input:\n" + lastInput})
		}
		if lastOutput, ok := lastExecutionStep.Response["llm_response"].(string); ok {
			fmt.Println("__________LAST OUTPUT : ________ ", lastOutput)
			messages = append(messages, llms.OpenAiChatCompletionMessage{Role: "assistant", Content: "your last output was:\n" + lastOutput})
		}
	}

	return messages
}

func (openAICodeGenerator *OpenAICodeGenerator) createInputContext(projectDir string) (string, error) {
	outputFile := projectDir + "/input_context.txt"
	allowedExtensions := []string{".py", ".html", ".css", ".txt", ".ini", ".jpg", ".png"}
	if err := openAICodeGenerator.ensureDirectoryExists(projectDir); err != nil {
		return "", err
	}
	err := openAICodeGenerator.generateFileListForInputContext(projectDir, outputFile, allowedExtensions)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(outputFile)
	if err != nil {
		return "", err
	}
	// Delete the outputFile after reading its content
	if err := os.Remove(outputFile); err != nil {
		fmt.Printf("Failed to delete the output file: %s, error: %s\n", outputFile, err)
	}

	return string(content), nil
}

func (openAICodeGenerator *OpenAICodeGenerator) generateFileListForInputContext(directory, outputFile string, allowedExtensions []string) error {
	outFile, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer outFile.Close()

	return filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Skip .venv directory and any other directories you want to exclude
		if info.IsDir() && (info.Name() == ".venv" || info.Name() == ".vscode" || info.Name() == "venv" || info.Name() == "frontend" || info.Name() == ".stories") {
			fmt.Printf("Skipping directory: %s\n", path)
			return filepath.SkipDir
		}

		if !info.IsDir() && openAICodeGenerator.fileExtensionAllowed(path, allowedExtensions) {
			if err := openAICodeGenerator.writeFileContent(path, directory, outFile); err != nil {
				return err
			}
		}

		return nil
	})
}

func (openAICodeGenerator *OpenAICodeGenerator) fileExtensionAllowed(file string, allowedExtensions []string) bool {
	for _, ext := range allowedExtensions {
		if strings.HasSuffix(file, ext) {
			return true
		}
	}
	return false
}

func (openAICodeGenerator *OpenAICodeGenerator) writeFileContent(filePath string, basePath string, outFile *os.File) error {
	absolutePath, err := filepath.Abs(filePath)
	if err != nil {
		return err
	}
	_, err = outFile.WriteString(fmt.Sprintf("|filename| : %s\n", absolutePath))
	if err != nil {
		return err
	}
	if strings.HasSuffix(strings.ToLower(filePath), ".jpg") || strings.HasSuffix(strings.ToLower(filePath), ".png") {
		_, err = outFile.WriteString("|code| : \n")
	} else {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		_, err = outFile.WriteString("|code| : \n" + string(content) + "\n\n")
	}
	return err
}

func (openAICodeGenerator *OpenAICodeGenerator) getSystemPrompt(framework string, projectDir string) string {
	var filePath string
	switch framework {
	case "flask":
		filePath = "/go/prompts/python/ai_developer_flask.txt"
	case "django":
		filePath = "/go/prompts/python/ai_developer_django.txt"
	default:
		filePath = ""
	}

	fmt.Println("______FILEPATH: _______" + filePath)
	content, err := os.ReadFile(filePath)
	if err != nil {
		panic(fmt.Sprintf("failed to read system prompt from %s: %v", filePath, err))
	}

	modifiedContent := strings.Replace(string(content), "{project_workspace_id}", projectDir, -1)
	return modifiedContent
}

func (openAICodeGenerator *OpenAICodeGenerator) ensureDirectoryExists(dirPath string) error {
	_, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		fmt.Printf("Directory does not exist: %s\n", dirPath)
		return err
	}
	return err
}

func (openAICodeGenerator *OpenAICodeGenerator) buildFinalInstructionForGeneration(
	step steps.GenerateCodeStep) (string, error) {
	// Initialize the final instruction string
	finalInstruction, err := openAICodeGenerator.buildInstructionForFirstExecution(step)
	if step.Retry {
		finalInstruction, err = openAICodeGenerator.buildInstructionOnRetry(step)
		if err != nil {
			fmt.Printf("Error building instruction on retry: %s\n", err.Error())
			return "", err
		}
	} else if step.Execution.ReExecution {
		finalInstruction, err = openAICodeGenerator.buildInstructionOnReExecutionWithComments(step)
		if err != nil {
			fmt.Printf("Error building instruction on re-execution: %s\n", err.Error())
			return "", err
		}
	}

	return finalInstruction, nil
}

func (openAICodeGenerator *OpenAICodeGenerator) buildInstructionOnReExecutionWithComments(step steps.GenerateCodeStep) (string, error) {
	fmt.Printf("Building instruction on re-execution with comments for step: %s\n", step.StepName())
	fmt.Printf("Pull Request ID is %d\n", step.PullRequestID)
	comments, err := openAICodeGenerator.pullRequestCommentService.GetAllCommentsByPullRequestID(step.PullRequestID)
	if err != nil {
		fmt.Printf("Error fetching comments: %s\n", err.Error())
		return "", err
	}
	finalInstruction := ""
	if len(comments) > 0 {
		finalInstruction = comments[len(comments)-1].Comment
	}
	return finalInstruction, nil
}

func (openAICodeGenerator *OpenAICodeGenerator) buildInstructionForFirstExecution(step steps.GenerateCodeStep) (string, error) {
	instructions, err := openAICodeGenerator.storyService.GetStoryInstructionByStoryId(int(step.Story.ID))
	if err != nil {
		fmt.Printf("Error fetching instructions: %s\n", err.Error())
		return "", err
	}
	testCases, err := openAICodeGenerator.storyService.GetStoryTestCaseByStoryId(int(step.Story.ID))
	if err != nil {
		fmt.Printf("Error fetching test cases: %s\n", err.Error())
		return "", err
	}

	fmt.Printf("Building instruction for first execution\n")
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Title: %s\n", step.Story.Title))
	sb.WriteString(fmt.Sprintf("Description: %s\n", step.Story.Description))
	sb.WriteString("Instructions: ")
	for _, instruction := range instructions {
		sb.WriteString(instruction.Instruction + " ")
	}
	sb.WriteString("\n")
	sb.WriteString("Test cases: ")
	for _, testCase := range testCases {
		sb.WriteString(testCase.TestCase + " ")
	}
	sb.WriteString("\n")
	return sb.String(), nil
}

func (openAICodeGenerator *OpenAICodeGenerator) buildInstructionOnRetry(step steps.GenerateCodeStep) (string, error) {
	fmt.Printf("Building instruction on retry for step: %s\n", step.StepName())
	previousServerTestExecutionStep, err := openAICodeGenerator.executionStepService.FetchExecutionSteps(
		step.Execution.ID,
		steps.SERVER_START_STEP.String(),
		steps.CODE_TEST.String(),
		1,
	)
	if err != nil {
		fmt.Printf("Error fetching previous server test execution step: %s\n", err.Error())
		return "", err
	}
	finalInstruction := ""
	if len(previousServerTestExecutionStep) > 0 {
		finalInstruction = previousServerTestExecutionStep[0].Response["error"].(string)
	}
	return finalInstruction, nil
}