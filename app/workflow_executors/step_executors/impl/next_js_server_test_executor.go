package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/llms"
	"ai-developer/app/services"
	"ai-developer/app/utils"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"bytes"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"os"
	"os/exec"
	"strings"
)

type NextJsServerStartTestExecutor struct {
	executionStepService *services.ExecutionStepService
	activityLogService   *services.ActivityLogService
	logger               *zap.Logger
	claudeClient         *llms.ClaudeClient
	executionService     *services.ExecutionService
	llmAPIKeyService     *services.LLMAPIKeyService
	storyService         *services.StoryService
	projectService       *services.ProjectService
}

func NewNextJsServerStartTestExecutor(
	executionStepService *services.ExecutionStepService,
	activityLogService *services.ActivityLogService,
	logger *zap.Logger,
	claudeClient *llms.ClaudeClient,
	llmAPIKeyService *services.LLMAPIKeyService,
	executionService *services.ExecutionService,
	storyService *services.StoryService,
	projectService *services.ProjectService,
) *NextJsServerStartTestExecutor {
	return &NextJsServerStartTestExecutor{
		executionStepService: executionStepService,
		activityLogService:   activityLogService,
		logger:               logger,
		claudeClient:         claudeClient,
		llmAPIKeyService:     llmAPIKeyService,
		executionService:     executionService,
		storyService:         storyService,
		projectService:       projectService,
	}
}

func (e NextJsServerStartTestExecutor) Execute(step steps.ServerStartTestStep) error {
	fmt.Printf("Executing Server Start Test Step: %s\n", step.StepName())
	codeFolder := config.WorkspaceWorkingDirectory() + "/" + step.Project.HashID + "/" + step.Story.HashID
	err := e.activityLogService.CreateActivityLog(
		step.ExecutionStep.ExecutionID,
		step.ExecutionStep.ID,
		"INFO",
		fmt.Sprintf("Starting and testing Server..."),
	)
	if err != nil {
		fmt.Println("Error creating activity log" + err.Error())
		return err
	}

	buildLogs, err := e.serverRunTest(codeFolder, step.ExecutionStep.ExecutionID, step.ExecutionStep.ID, step.Story.HashID, step.Project.HashID)
	if err != nil {
		return err
	}
	directoryPlan, err := utils.GetDirectoryStructure(codeFolder)
	if err != nil {
		return err
	}

	story, err := e.storyService.GetStoryByExecutionID(step.Execution.ID)
	if err != nil {
		fmt.Println("Error getting story by execution ID: ", err)
	}
	projectId := story.ProjectID
	project, err := e.projectService.GetProjectById(projectId)
	if err != nil {
		fmt.Println("Error getting project by ID: ", err)
	}
	organisationId := project.OrganisationID
	fmt.Println("_________ORGANISATION ID_________", organisationId)
	if e.llmAPIKeyService == nil {
		fmt.Println("_____NULL_____")
	}
	llmAPIKey, err := e.llmAPIKeyService.GetLLMAPIKeyByModelName("claude-3", organisationId)
	if err != nil {
		fmt.Println("Error getting openai api key: ", err)
	}
	apiKey := llmAPIKey.LLMAPIKey
	fmt.Println("_________API KEY_________", apiKey)

	buildAnalysis, action, err := e.AnalyseBuildLogs(buildLogs, directoryPlan, apiKey)
	fmt.Println("Build Logs Analysis", buildAnalysis)
	if err != nil {
		fmt.Println("Error analysing build log" + err.Error())
		return err
	}
	if !buildAnalysis {
		fmt.Println("Editing Code with action", action)
		if _, ok := action["command"]; !ok {
			action["command"] = ""
		}
		if _, ok := action["cwd"]; !ok {
			action["cwd"] = ""
		}
		if _, ok := action["description"]; !ok {
			action["description"] = ""
		}
		if _, ok := action["file_path"]; !ok {
			action["file_path"] = ""
		}
		fmt.Println("Updating Execution Step Response after checking build analysis")
		err = e.executionStepService.UpdateExecutionStepResponse(
			step.ExecutionStep,
			map[string]interface{}{
				"actionType":  action["type"].(string),
				"fileName":    action["file_path"].(string),
				"command":     action["command"].(string),
				"cwd":         action["cwd"].(string),
				"description": action["description"].(string),
			},
			"SUCCESS",
		)
		if err != nil {
			fmt.Println("Error updating execution step response" + err.Error())
			return err
		}
		fmt.Println("Retrying Code Generation")
		return fmt.Errorf("%w: %v", steps.ErrReiterate, err)
	} else {

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
}

func (e NextJsServerStartTestExecutor) AnalyseBuildLogs(buildLogs, directoryPlan, apiKey string) (bool, map[string]interface{}, error) {
	fmt.Println("Analysing Build Logs", buildLogs)
	messages, err := e.CreateMessage(buildLogs, directoryPlan)
	if err != nil {
		return false, nil, err
	}
	e.claudeClient.WithApiKey(apiKey)
	response, err := e.claudeClient.ChatCompletion(messages)
	if err != nil {
		fmt.Println("failed to generate code from OpenAI API")
		return false, nil, fmt.Errorf("failed to generate code from OpenAI API: %w", err)
	}
	var jsonResponse map[string]interface{}
	if err = json.Unmarshal([]byte(response), &jsonResponse); err != nil {
		fmt.Println("failed to unmarshal response from Claude API, Failed to parse response as JSON on attempt.")
		return false, nil, fmt.Errorf("failed to unmarshal response from Claude API: %w", err)
	}
	fmt.Println("Response after extracting JSON: ", jsonResponse)
	buildResponse, action := e.CheckBuildResponse(jsonResponse)
	fmt.Println("Build Logs Check Response")
	return buildResponse, action, nil

}

func (e NextJsServerStartTestExecutor) CheckBuildResponse(response map[string]interface{}) (bool, map[string]interface{}) {
	buildSuccessful, ok := response["build_successful"].(string)
	if !ok {
		buildSuccessful = "No"
	}

	if buildSuccessful == "Yes" {
		return true, nil
	}

	action, ok := response["action"].(map[string]interface{})
	if !ok {
		action = make(map[string]interface{})
	}

	return false, action
}

func (e NextJsServerStartTestExecutor) CreateMessage(buildLogs string, directoryPlan string) ([]llms.ClaudeChatCompletionMessage, error) {
	replacements := map[string]string{
		"BUILD_LOGS":          buildLogs,
		"DIRECTORY_STRUCTURE": directoryPlan,
	}
	content, err := os.ReadFile("/go/prompts/next_js_build_checker.txt")
	var systemPrompt string
	for key, value := range replacements {
		systemPrompt = strings.ReplaceAll(string(content), key, value)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load system prompt: %w", err)
	}

	messages := []llms.ClaudeChatCompletionMessage{
		{
			Role: "user",
			Content: []llms.MessageContent{
				{
					Type: "text",
					Text: systemPrompt,
				},
			},
		},
	}

	return messages, nil
}

func (e NextJsServerStartTestExecutor) serverRunTest(codeFolder string, executionId, executionStepId uint, storyHashID string, projectHashID string) (string, error) {
	var stdout, stderr bytes.Buffer

	// Function to run a command and capture its output
	runCommand := func(name string, arg ...string) error {
		cmd := exec.Command(name, arg...)
		cmd.Dir = codeFolder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		basePath := "/" + projectHashID + "/" + storyHashID + "/out"
		cmd.Env = append(os.Environ(), "NEXT_PUBLIC_BASE_PATH="+basePath)
		fmt.Println(cmd.Env)
		if err := cmd.Run(); err != nil {
			fmt.Printf("failed to run command %s %v: %v", name, arg, err.Error())
		}
		err := e.activityLogService.CreateActivityLog(
			executionId,
			executionStepId,
			"CODE",
			fmt.Sprintf(stdout.String()+stderr.String()),
		)
		if err != nil {
			return err
		}
		fmt.Println(stdout.String(), stderr.String())
		return nil
	}

	// Run the necessary commands
	fmt.Printf("Building Next App in %s\n", codeFolder)
	err := runCommand("npm", "i")
	if err != nil {
		return "", err
	}
	err = runCommand("npm", "install", "react-icons", "--save")
	if err != nil {
		return "", err
	}
	err = runCommand("npm", "run", "build")
	if err != nil {
		return "", err
	}
	fmt.Println("Successfully built Next App")
	return stdout.String() + stderr.String(), nil
}
