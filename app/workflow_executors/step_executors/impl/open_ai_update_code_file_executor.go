package impl

import (
	"ai-developer/app/services"
	"ai-developer/app/utils"
	"ai-developer/app/workflow_executors/step_executors/steps"
	"encoding/json"
	"fmt"
	"strings"
)

type UpdateCodeFileExecutor struct {
	executionStepService *services.ExecutionStepService
	activityLogService   *services.ActivityLogService
}

func NewUpdateCodeFileExecutor(
	executionStepService *services.ExecutionStepService,
	activeLogService *services.ActivityLogService,
) *UpdateCodeFileExecutor {
	return &UpdateCodeFileExecutor{
		executionStepService: executionStepService,
		activityLogService:   activeLogService,
	}
}

func (e UpdateCodeFileExecutor) Execute(step steps.UpdateCodeFileStep) error {

	err := e.activityLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Updating code files...")
	if err != nil {
		fmt.Println("Error creating activity log" + err.Error())
		return err
	}

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
	fmt.Println("___________Updating Code____________________")
	fmt.Println("Steps:", generateCodeSteps)
	// Convert JSONMap to a JSON string
	responseJSON, err := json.Marshal(generateCodeSteps[0].Response)
	if err != nil {
		fmt.Println("Error marshalling JSONMap" + err.Error())
		return fmt.Errorf("failed to marshal JSONMap: %w", err)
	}
	// Unmarshal the JSON string to the response struct
	var response struct {
		LLMResponse string `json:"llm_response"`
	}
	if err := json.Unmarshal(responseJSON, &response); err != nil {
		return fmt.Errorf("failed to unmarshal llm_response: %w", err)
	}
	fmt.Println("Updating code file...")
	llmResponse := response.LLMResponse
	lines := strings.Split(llmResponse, "\n")
	var currentFilename string
	var currentContent []string
	isCode := false
	fmt.Println("Processing lines...")
	for _, line := range lines {
		if strings.HasPrefix(line, "|filename|") {
			if currentFilename != "" {
				err := utils.WriteToFile(currentFilename, currentContent)
				if err != nil {
					return err
				}
			}
			currentFilename = strings.TrimSpace(strings.Split(line, ":")[1])
			currentContent = []string{}
			isCode = false
		} else if strings.HasPrefix(line, "|code|") || strings.HasPrefix(line, "|terminal|") {
			isCode = true
		} else if isCode {
			if strings.TrimSpace(line) == "```" || strings.TrimSpace(line) == "```shell" || strings.TrimSpace(line) == "```plaintext" || strings.TrimSpace(line) == "```bash" || strings.TrimSpace(line) == "```terminal" || strings.TrimSpace(line) == "```python" || strings.TrimSpace(line) == "```css" || strings.TrimSpace(line) == "```html" || strings.TrimSpace(line) == "```javascript" || strings.TrimSpace(line) == "```ini" {
				continue
			}
			currentContent = append(currentContent, line)
		}
	}

	if currentFilename != "" {
		err := utils.WriteToFile(currentFilename, currentContent)
		if err != nil {
			return err
		}
	}

	err = e.activityLogService.CreateActivityLog(step.Execution.ID, step.ExecutionStep.ID, "INFO", "Updated code files.")
	if err != nil {
		fmt.Println("Error creating activity log" + err.Error())
		return err
	}
	return nil
}
