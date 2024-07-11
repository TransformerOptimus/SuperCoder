package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/constants"
	"ai-developer/app/llms"
	"ai-developer/app/models"
	"ai-developer/app/services"
	"ai-developer/app/services/s3_providers"
	"ai-developer/app/utils"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type OpenAiNextJsCodeGenerator struct {
	claudeClient         *llms.ClaudeClient
	projectService       *services.ProjectService
	executionStepService *services.ExecutionStepService
	executionService     *services.ExecutionService
	storyService         *services.StoryService
	activityLogService   *services.ActivityLogService
	designReviewService  *services.DesignStoryReviewService
	s3Service            *s3_providers.S3Service
	llmAPIKeyService     *services.LLMAPIKeyService
}

func NewOpenAINextJsCodeGenerationExecutor(
	claudeClient *llms.ClaudeClient,
	projectService *services.ProjectService,
	executionStepService *services.ExecutionStepService,
	executionService *services.ExecutionService,
	storyService *services.StoryService,
	activityLogService *services.ActivityLogService,
	designReviewService *services.DesignStoryReviewService,
	s3Service *s3_providers.S3Service,
	llmAPIKeyService *services.LLMAPIKeyService,
) *OpenAiNextJsCodeGenerator {
	return &OpenAiNextJsCodeGenerator{
		claudeClient:         claudeClient,
		projectService:       projectService,
		executionStepService: executionStepService,
		executionService:     executionService,
		storyService:         storyService,
		activityLogService:   activityLogService,
		designReviewService:  designReviewService,
		s3Service:            s3Service,
		llmAPIKeyService:     llmAPIKeyService,
	}
}

func (openAiCodeGenerator OpenAiNextJsCodeGenerator) Execute(step steps.GenerateCodeStep) error {
	fmt.Printf("Executing GenerateCodeStep: %s\n", step.StepName())
	fmt.Printf("Working on project details: %v\n", step.Project)
	fmt.Printf("Working on story details:  %v\n", step.Story)
	fmt.Printf("Max loop iterations: %d\n", step.MaxLoopIterations)
	fmt.Printf("Is re-execution: %v\n", step.Execution.ReExecution)
	fmt.Printf("Is Retry : %v\n", step.Retry)
	fmt.Printf("File Name: %s\n", step.File)

	storyDir := config.WorkspaceWorkingDirectory() + "/stories/" + step.Project.HashID + "/" + step.Story.HashID + "/app"
	fmt.Println("____________Project Directory: ", storyDir)
	fmt.Println("___________Checking for Max Retry______________")
	count, err := openAiCodeGenerator.executionStepService.CountExecutionStepsOfName(step.Execution.ID, steps.CODE_GENERATE_STEP.String())
	if err != nil {
		fmt.Printf("Error checking max retry for generation: %s\n", err.Error())
		return err
	}
	fmt.Printf("Count of LLM steps: %d\n", count)

	if count > step.MaxLoopIterations {
		fmt.Println("Max retry limit reached for LLM steps")
		//Update story status to MAX_LOOP_ITERATION_REACHED
		if err = openAiCodeGenerator.storyService.UpdateStoryStatus(int(step.Story.ID), constants.MaxLoopIterationReached); err != nil {
			fmt.Printf("Error updating story status: %s\n", err.Error())
			return err
		}
		//Update execution status to MAX_LOOP_ITERATION_REACHED
		if err = openAiCodeGenerator.executionService.UpdateExecutionStatus(step.Execution.ID, constants.MaxLoopIterationReached); err != nil {
			fmt.Printf("Error updating execution step: %s\n", err.Error())
			return err
		}

		err = openAiCodeGenerator.activityLogService.CreateActivityLog(
			step.Execution.ID,
			step.ExecutionStep.ID,
			"ERROR",
			"Max retry limit reached for code generation!",
		)
		if err != nil {
			fmt.Printf("Error creating activity log: %s\n", err.Error())
			return err
		}

		return fmt.Errorf("max retry limit reached for LLM steps")
	}

	finalInstructionForGeneration, err := openAiCodeGenerator.buildFinalInstructionForGeneration(step, storyDir)
	if err != nil {
		fmt.Printf("Error building final instruction for generation: %s\n", err.Error())
		return err
	}
	//extracting api_key from executionId
	story, err := openAiCodeGenerator.storyService.GetStoryByExecutionID(step.Execution.ID)
	if err != nil {
		fmt.Println("Error getting story by execution ID: ", err)
	}
	projectId := story.ProjectID
	project, err := openAiCodeGenerator.projectService.GetProjectById(projectId)
	if err != nil {
		fmt.Println("Error getting project by ID: ", err)
	}
	organisationId := project.OrganisationID
	fmt.Println("_________ORGANISATION ID_________", organisationId)
	if openAiCodeGenerator.llmAPIKeyService == nil {
		fmt.Println("_____NULL_____")
	}
	llmAPIKey, err := openAiCodeGenerator.llmAPIKeyService.GetLLMAPIKeyByModelName("claude-3", organisationId)
	if err != nil {
		fmt.Println("Error getting openai api key: ", err)
	}
	apiKey := llmAPIKey.LLMAPIKey
	fmt.Println("_________API KEY_________", apiKey)

	code, err := openAiCodeGenerator.GenerateCode(step, finalInstructionForGeneration, storyDir, apiKey)
	if err != nil {
		fmt.Println("____ERROR OCCURRED WHILE GENERATING CODE: ______", err)
		settingsUrl := config.Get("app.url").(string) + "/settings"
		err = openAiCodeGenerator.activityLogService.CreateActivityLog(
			step.Execution.ID,
			step.ExecutionStep.ID,
			"INFO",
			fmt.Sprintf("Action required: There's an issue with your LLM API Key. Ensure your API Key is correct. <a href='%s' style='color:%s; text-decoration:%s'>Settings</a>", settingsUrl, "blue", "underline"),
		)
		if err != nil {
			fmt.Printf("Error creating activity log: %s\n", err.Error())
			return err
		}
		//Update Execution Status and Story Status
		if err = openAiCodeGenerator.storyService.UpdateStoryStatus(int(step.Story.ID), constants.InReview); err != nil {
			fmt.Printf("Error updating story status: %s\n", err.Error())
			return err
		}
		if err = openAiCodeGenerator.executionService.UpdateExecutionStatus(step.Execution.ID, constants.InReview); err != nil {
			fmt.Printf("Error updating execution step: %s\n", err.Error())
			return err
		}
		return err
	}
	fmt.Printf("_________Generated Code__________: %s\n", code)

	if err = openAiCodeGenerator.executionStepService.UpdateExecutionStepResponse(
		step.ExecutionStep,
		map[string]interface{}{
			"file_name":    finalInstructionForGeneration["fileName"],
			"llm_response": code,
		},
		"SUCCESS"); err != nil {
		fmt.Printf("Error updating execution step: %s\n", err.Error())
		return err
	}

	err = openAiCodeGenerator.activityLogService.CreateActivityLog(
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

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) buildFinalInstructionForGeneration(
	step steps.GenerateCodeStep, storyDir string) (map[string]string, error) {
	// Initialize the final instruction string
	finalInstruction, err := openAiCodeGenerator.buildInstructionForFirstExecution(step, storyDir)
	if err!= nil {
        fmt.Printf("Error building instruction for first execution: %s\n", err.Error())
        return nil, err
    }
	if step.Retry {
		fmt.Println("Building instruction on retry limit reached for LLM steps")
		finalInstruction, err = openAiCodeGenerator.buildInstructionOnRetry(step, storyDir)
		if err != nil {
			fmt.Printf("Error building instruction on retry: %s\n", err.Error())
			return nil, err
		}
	} else if step.Execution.ReExecution {
		finalInstruction, err = openAiCodeGenerator.buildInstructionOnReExecutionWithComments(step, storyDir)
		if err != nil {
			fmt.Printf("Error building instruction on re-execution: %s\n", err.Error())
			return nil, err
		}
	}

	// Print the final instruction
	//fmt.Println("Final Instruction:", finalInstruction)
	return finalInstruction, nil
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) buildInstructionForFirstExecution(step steps.GenerateCodeStep, storyDir string) (map[string]string, error) {
	err := openAiCodeGenerator.activityLogService.CreateActivityLog(
		step.Execution.ID,
		step.ExecutionStep.ID,
		"INFO",
		fmt.Sprintf("Code generation has started for file: %s", step.File),
	)
	if err != nil {
		fmt.Printf("Error creating activity log: %s\n", err.Error())
		return nil, err
	}
	fmt.Printf("Building instruction for first execution\n")
	storyFile, err := openAiCodeGenerator.storyService.GetStoryFileByStoryId(step.Story.ID)
	//filePath := filepath.Join(storyDir, step.File)
	//code, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	base64Image, imageType, err := openAiCodeGenerator.s3Service.GetBase64FromS3Url(storyFile.FilePath)
	if err != nil {
		return nil, err
	}

	code, err := openAiCodeGenerator.getFilesContent(storyDir)
	if err!=nil{
		return nil, err
	}

	return map[string]string{
		"existingCode": string(code),
		"base64Image":  base64Image,
		"fileName":     step.File,
		"feedback":     "Try to replicate original screenshot.",
		"imageType":    imageType,
	}, nil
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) getFilesContent(folderPath string) (string, error) {
	files, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Printf("Error reading directory %s\n", folderPath)
		return "", err
	}

	var fileData string
	
	for _, file := range files {
		if !file.IsDir() && (strings.HasSuffix(file.Name(), ".css") || strings.HasSuffix(file.Name(), ".tsx")) {
			fullPath := filepath.Join(folderPath, file.Name())
			content, err := os.ReadFile(fullPath)
			if err != nil {
				fmt.Printf("Error reading file %s: %s\n", fullPath, err)
				return "", err
			}
			fileData += string("Code for:"+file.Name()+":\n"+ string(content)+"\n\n")
		}
	}

	return fileData, nil
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) buildInstructionOnRetry(step steps.GenerateCodeStep, storyDir string) (map[string]string, error) {
	fmt.Printf("Building instruction on retry for step: %s\n", step.StepName())
	previousServerTestExecutionStep, err := openAiCodeGenerator.executionStepService.FetchExecutionSteps(
		step.Execution.ID,
		steps.SERVER_START_STEP.String(),
		steps.CODE_TEST.String(),
		1,
	)
	if err != nil {
		return nil, err
	}

	fmt.Println("---Response from GPT in case of NPM Build Failure---", previousServerTestExecutionStep[0].Response)

	fileName := previousServerTestExecutionStep[0].Response["fileName"].(string)

	err = openAiCodeGenerator.activityLogService.CreateActivityLog(
		step.Execution.ID,
		step.ExecutionStep.ID,
		"INFO",
		fmt.Sprintf("Code generation has started for file: %s", fileName),
	)
	if err != nil {
		fmt.Printf("Error creating activity log: %s\n", err.Error())
		return nil, err
	}

	storyDir = strings.TrimSuffix(storyDir, "app")
	fmt.Println("stroy dir----", storyDir)
	fmt.Println("filename-----", fileName)
	filePath := filepath.Join(storyDir, fileName)
	code, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Println("Error getting code: ", err.Error())
		return nil, err
	}

	return map[string]string{
		"actionType":   previousServerTestExecutionStep[0].Response["actionType"].(string),
		"fileName":     fileName,
		"command":      previousServerTestExecutionStep[0].Response["command"].(string),
		"cwd":          previousServerTestExecutionStep[0].Response["cwd"].(string),
		"description":  previousServerTestExecutionStep[0].Response["description"].(string),
		"existingCode": string(code),
	}, nil
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) buildInstructionOnReExecutionWithComments(step steps.GenerateCodeStep, storyDir string) (map[string]string, error) {
	fmt.Printf("Building instruction on re-execution with comments for step: %s\n", step.StepName())
	comments, err := openAiCodeGenerator.designReviewService.GetAllDesignReviewsByStoryId(step.Story.ID)
	if err != nil {
		fmt.Printf("Error fetching comments: %s\n", err.Error())
		return nil, err
	}
	feedback := ""
	if len(comments) > 0 {
		feedback = comments[len(comments)-1].Comment
	}
	fmt.Println("Feedback:", feedback)
	//storyDir = config.WorkspaceWorkingDirectory() + "/story/" + step.Story.HashID
	filePath := filepath.Join(storyDir, step.File)
	code, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	storyFile, err := openAiCodeGenerator.storyService.GetStoryFileByStoryId(step.Story.ID)
	if err != nil {
		return nil, err
	}
	base64Image, imageType, err := openAiCodeGenerator.s3Service.GetBase64FromS3Url(storyFile.FilePath)
	if err != nil {
		return nil, err
	}

	return map[string]string{
		"existingCode": string(code),
		"base64Image":  base64Image,
		"imageType":    imageType,
		"fileName":     step.File,
		"feedback":     feedback,
	}, nil
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) GenerateCode(step steps.GenerateCodeStep, instruction map[string]string, storyDir string, apiKey string) (string, error) {
	if step.Retry {
		response, err := openAiCodeGenerator.GenerateCodeOnRetry(step.ExecutionStep, instruction, storyDir)
		if err != nil {
			fmt.Println("Error generating code on retry")
			return "", err
		}
		return response, nil
	} else {
		messages, err := openAiCodeGenerator.GenerateMessages(instruction, storyDir, step)
		if err != nil {
			return "", err
		}
		err = openAiCodeGenerator.executionStepService.UpdateExecutionStepRequest(
			step.ExecutionStep,
			map[string]interface{}{
				"final_instruction": instruction,
				"llm_request":       messages,
			},
			"IN_PROGRESS",
		)
		openAiCodeGenerator.claudeClient.WithApiKey(apiKey)
		response, err := openAiCodeGenerator.claudeClient.ChatCompletion(messages)
		if err != nil {
			return "", fmt.Errorf("failed to generate code from Claude API: %w", err)
		}
		response = openAiCodeGenerator.ProcessMessageResponse(response)
		return response, nil
	}
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) ProcessMessageResponse(messages string) string {
	if strings.HasPrefix(messages, "```") {
		messages = messages[3 : len(messages)-3]
		lines := strings.Split(messages, "\n")
		if len(lines) > 1 {
			messages = strings.Join(lines[1:], "\n")
		} else {
			messages = "" // Handle case where there are no lines after the first
		}
	}
	return messages

}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) GenerateCodeOnRetry(executionStep *models.ExecutionStep, instruction map[string]string, storyDir string) (string, error) {
	switch instruction["actionType"] {
	case "create":
		filePath := storyDir + instruction["fileName"]
		fmt.Printf("Creating new file at %s\n", filePath)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			fmt.Printf("Error creating directory: %v\n", err)
			return "", err
		}
		if _, err := os.Create(filePath); err != nil {
			fmt.Printf("Error creating file: %v\n", err)
			return "", err
		}
		return "", nil
	case "terminal":
		command := instruction["command"]
		cwd := filepath.Join(storyDir, instruction["cwd"])
		fmt.Printf("Executing terminal command: %s\n", command)
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = cwd
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error executing command: %v\n", err)
			return "", err
		}
		return "", nil
	case "edit":
		response, err := openAiCodeGenerator.EditCodeOnRetry(instruction, storyDir, executionStep)
		if err != nil {
			return "", err
		}
		return response, nil
	default:
		fmt.Printf("Unknown action type: %s\n", instruction["actionType"])
		return "", fmt.Errorf("unknown action type: %s", instruction["actionType"])
	}
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) EditCodeOnRetry(instruction map[string]string, storyDir string, executionStep *models.ExecutionStep) (string, error) {
	generationPlan, err := openAiCodeGenerator.GetCodeGenerationPlan(storyDir)
	if err != nil {
		return "", err
	}
	systemPrompt, err := openAiCodeGenerator.GetRetrySystemPrompt(instruction, generationPlan)
	if err != nil {
		return "", err
	}
	messages := openAiCodeGenerator.GetMessagesOnRetry(systemPrompt, instruction["description"])
	err = openAiCodeGenerator.executionStepService.UpdateExecutionStepRequest(
		executionStep,
		map[string]interface{}{
			"final_instruction": instruction,
			"llm_request":       messages,
		},
		"IN_PROGRESS",
	)
	response, err := openAiCodeGenerator.claudeClient.ChatCompletion(messages)
	if err != nil {
		return "", fmt.Errorf("failed to generate code from OpenAI API: %w", err)
	}
	return response, nil
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) GenerateMessages(instruction map[string]string, storyDir string, step steps.GenerateCodeStep) ([]llms.ClaudeChatCompletionMessage, error) {
	generationPlan, err := openAiCodeGenerator.GetCodeGenerationPlan(storyDir)
	if err != nil {
		return nil, err
	}
	systemPrompt, err := openAiCodeGenerator.getSystemPrompt(instruction, step)
	if err != nil {
		return nil, err
	}
	messages := openAiCodeGenerator.GetMessages(systemPrompt, instruction, generationPlan)
	return messages, nil
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) GetMessages(systemPrompt string, instruction map[string]string, generationPlan string) []llms.ClaudeChatCompletionMessage {
	// fmt.Println(instruction["feedback"])
	// fmt.Println(instruction["existingCode"])
	messages := []llms.ClaudeChatCompletionMessage{
		{
			Role: "user",
			Content: []llms.MessageContent{
				{
					Type: "text",
					Text: "The original screenshot is:",
				},
				{
					Type: "image",
					Source: &llms.ImageSourceData{
						Type:      "base64",
						MediaType: instruction["imageType"],
						Data:      instruction["base64Image"],
					},
				},
				{
					Type: "text",
					Text: fmt.Sprintf("%s\n The directory structure and the tech stack is as follow as: \n%s", systemPrompt, generationPlan),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("User Feedback: %s", instruction["feedback"]),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Existing Code:\n%s for the file: %s\n", instruction["existingCode"], instruction["fileName"]),
				},
				{
					Type: "text",
					Text: fmt.Sprintf("Code written in files to incorporate the feedback: %s\n", instruction["feedback"]),
				},
			},
		},
	}

	return messages
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) GetMessagesOnRetry(systemPrompt string, errorDescription string) []llms.ClaudeChatCompletionMessage {
	messages := []llms.ClaudeChatCompletionMessage{
		{
			Role: "user",
			Content: []llms.MessageContent{
				{
					Type: "text",
					Text: fmt.Sprintf("%s\nHere is the error description: %s", systemPrompt, errorDescription),
				},
			},
		},
	}

	return messages
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) getSystemPrompt(instruction map[string]string, step steps.GenerateCodeStep) (string, error) {
	content, err := os.ReadFile("/go/prompts/ai_frontend_developer.txt")
	if err != nil {
		panic(fmt.Sprintf("failed to read system prompt: %v", err))
	}
	systemPrompt := strings.Replace(string(content), "{{EXISTING_CODE}}", instruction["existingCode"], -1)
	systemPrompt = strings.Replace(string(systemPrompt), "{{USER_FEEDBACK}}", instruction["feedback"], -1)
	systemPrompt = strings.Replace(string(systemPrompt), "{{FILE_NAME}}", step.File, -1)
	return systemPrompt, nil
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) GetRetrySystemPrompt(instruction map[string]string, directoryStructure string) (string, error) {
	// replacements := map[string]string{
	// 	"FILE_NAME":           instruction["fileName"],
	// 	"ERROR_DESCRIPTION":   instruction["description"],
	// 	"DIRECTORY_STRUCTURE": directoryStructure,
	// 	"CURRENT_CODE":        instruction["existingCode"],
	// }
	content, err := os.ReadFile("/go/prompts/ai_frontend_developer_edit_code.txt")
	if err != nil {
		panic(fmt.Sprintf("failed to read system prompt: %v", err))
		return "", err
	}
	modifiedContent := strings.Replace(string(content), "{{FILE_NAME}}", instruction["fileName"], -1)
	modifiedContent = strings.Replace(string(modifiedContent), "{{ERROR_DESCRIPTION}}", instruction["description"], -1)
	modifiedContent = strings.Replace(string(modifiedContent), "{{DIRECTORY_STRUCTURE}}", directoryStructure, -1)
	modifiedContent = strings.Replace(string(modifiedContent), "{{CURRENT_CODE}}", instruction["existingCode"], -1)
	// var systemPrompt string
	// for key, value := range replacements {
	// 	systemPrompt = strings.ReplaceAll(string(content), key, value)
	// }
	return modifiedContent, nil
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) GetCodeGenerationPlan(storyDir string) (string, error) {
	if err := openAiCodeGenerator.ensureDirectoryExists(storyDir); err != nil {
		return "", err
	}

	directoryStructure, err := utils.GetDirectoryStructure(storyDir)
	if err != nil {
		return "", err
	}
	return directoryStructure, nil
}

func (openAiCodeGenerator *OpenAiNextJsCodeGenerator) ensureDirectoryExists(dirPath string) error {
	_, err := os.Stat(dirPath)
	if os.IsNotExist(err) {
		fmt.Printf("Directory does not exist: %s\n", dirPath)
		return err
	}
	return err
}
