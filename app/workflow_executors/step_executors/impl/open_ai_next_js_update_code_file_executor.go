package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/services"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type NextJsUpdateCodeFileExecutor struct {
	executionStepService *services.ExecutionStepService
	activityLogService   *services.ActivityLogService
}

func NewNextJsUpdateCodeFileExecutor(
	executionStepService *services.ExecutionStepService,
	activeLogService *services.ActivityLogService,
) *NextJsUpdateCodeFileExecutor {
	return &NextJsUpdateCodeFileExecutor{
		executionStepService: executionStepService,
		activityLogService:   activeLogService,
	}
}

type Response struct {
	LLMResponse string `json:"llm_response"`
	FileName    string `json:"file_name"`
}

func (e NextJsUpdateCodeFileExecutor) Execute(step steps.UpdateCodeFileStep) error {
	fmt.Println("Updating code file for next js: ")
	generateCodeSteps, err := e.executionStepService.FetchExecutionSteps(
		step.Execution.ID,
		steps.CODE_GENERATE_STEP.String(),
		steps.LLM.String(),
		1,
	)
	if err != nil {
		fmt.Println("Error fetching execution generateCodeSteps" + err.Error())
		return fmt.Errorf("failed to fetch execution generateCodeSteps: %w", err)
	}
	if len(generateCodeSteps) == 0 {
		fmt.Println("No execution generateCodeSteps found for execution ID:", step.Execution.ID)
		return fmt.Errorf("no execution generateCodeSteps found for execution ID: %d", step.Execution.ID)
	}

	// Convert JSONMap to a JSON string
	responseJSON, err := json.Marshal(generateCodeSteps[0].Response)
	if err != nil {
		fmt.Println("Error marshalling JSONMap" + err.Error())
		return fmt.Errorf("failed to marshal JSONMap: %w", err)
	}
	var response Response
	// Convert JSONMap to a JSON string
	if err := json.Unmarshal(responseJSON, &response); err != nil {
		return fmt.Errorf("failed to unmarshal llm_response: %w", err)
	}
	llmResponse := response.LLMResponse
	fileName := response.FileName
	err = e.activityLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", fmt.Sprintf("Updating code file %s", fileName))
	if err != nil {
		fmt.Println("Error creating activity log" + err.Error())
		return err
	}
	if step.Retry {
		fmt.Println("___Response to UpdateCodeFile___ \n", response)
		err = e.UpdateReGeneratedCodeFile(response, step)
		if err != nil {
			fmt.Println("Error updating regenerated code: ", err.Error())
			return err
		}
	} else {
		err = e.UpdateCodeFile(llmResponse, fileName, step)
		if err != nil {
			return err
		}
	}
	fmt.Println("File Updated Successfully")
	return nil
}

func (e *NextJsUpdateCodeFileExecutor) UpdateReGeneratedCodeFile(response Response, step steps.UpdateCodeFileStep) error {
	var llmResponse map[string]interface{}
	var filePath string
	if strings.Contains(response.FileName, "app/") {
		filePath = config.WorkspaceWorkingDirectory() + "/stories/" + step.Project.HashID + "/" + step.Story.HashID + "/" + response.FileName
	} else {
		filePath = config.WorkspaceWorkingDirectory() + "/stories/" + step.Project.HashID + "/" + step.Story.HashID + "/app/" + response.FileName
	}
	err := json.Unmarshal([]byte(response.LLMResponse), &llmResponse)
	if err != nil {
		return nil
	}
	switch llmResponse["type"].(string) {
	case "edit", "update":
		newCode := llmResponse["new_code"].(string)
		var startLine int
		switch startLineVal := llmResponse["start_line"].(type) {
		case float64:
			startLine = int(startLineVal)
		case int:
			startLine = startLineVal
		}
		var endLine int
		switch endLineVal := llmResponse["end_line"].(type) {
		case float64:
			endLine = int(endLineVal)
		case int:
			endLine = endLineVal
		}
		err = e.EditCode(filePath, startLine, endLine, newCode)
		if err != nil {
			fmt.Println("Error editing code: ", err)
			return err
		}
	case "insert", "create":
		var lineNumber int
		switch lineVal := llmResponse["line_number"].(type) {
		case float64:
			lineNumber = int(lineVal)
		case int:
			lineNumber = lineVal
		}
		newCode := llmResponse["new_code"].(string)
		err := e.InsertCode(filePath, lineNumber, newCode)
		if err != nil {
			fmt.Printf("Error inserting code: %v\n", err)
			return err
		}
	default:
		fmt.Println("Unknown llmResponse:", llmResponse["type"].(string))
		return fmt.Errorf("unknown response type: %s", llmResponse["type"])
	}
	return nil
}

func (e *NextJsUpdateCodeFileExecutor) EditCode(filePath string, startLine, endLine int, newCode string) error {
	fmt.Println("Start Line:", startLine)
	fmt.Println("End Line:", endLine)
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening file", filePath, err.Error())
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err = scanner.Err(); err != nil {
		fmt.Println("Error scanning file:", err.Error())
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Adjust startLine and endLine for zero-based indexing
	startLine--
	endLine--

	newLines := strings.Split(newCode, "\n")
	fmt.Println("New Lines:", newLines)

	// Edge case handling when startLine and endLine are 0
	if startLine < 0 {
		startLine = 0
	}
	if endLine < 0 {
		endLine = 0
	}

	// Ensure endLine does not exceed the number of lines in the file
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}

	newContent := append(lines[:startLine], append(newLines, lines[endLine+1:]...)...)

	err = os.WriteFile(filePath, []byte(strings.Join(newContent, "\n")), 0644)
	if err != nil {
		fmt.Println("Error writing file:", err.Error())
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (e *NextJsUpdateCodeFileExecutor) InsertCode(filePath string, lineNumber int, newCode string) error {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening file", filePath, err.Error())
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Println("Error scanning file:", err.Error())
		return fmt.Errorf("failed to read file: %w", err)
	}

	lineNumber--
	newLines := strings.Split(newCode, "\n")
	newContent := append(lines[:lineNumber+1], append(newLines, lines[lineNumber+1:]...)...)

	err = os.WriteFile(filePath, []byte(strings.Join(newContent, "\n")), 0644)
	if err != nil {
		fmt.Println("Error writing file:", err.Error())
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (e NextJsUpdateCodeFileExecutor) UpdateCodeFile(llmResponse, fileName string, step steps.UpdateCodeFileStep) error {
	if strings.HasPrefix(llmResponse, "```") {
		llmResponse = llmResponse[3:] // Remove the first 3 characters (```)
		lines := strings.Split(llmResponse, "\n")
		if len(lines) > 0 {
			llmResponse = strings.Join(lines[1:], "\n") // Join all lines except the first one
		}
	}

	fmt.Println("___file name___",fileName)
	if step.File != "" {
		storyDir := config.WorkspaceWorkingDirectory() + "/stories/" + step.Project.HashID + "/" + step.Story.HashID + "/app/" + fileName
		err := os.WriteFile(storyDir, []byte(llmResponse), 0644)
		if err != nil {
			return err
		}
	}

	err := e.activityLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Updated code files.")
	if err != nil {
		fmt.Println("Error creating activity log" + err.Error())
		return err
	}
	return nil
}
