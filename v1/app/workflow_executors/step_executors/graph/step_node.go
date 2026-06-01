package graph

import (
	"ai-developer/app/workflow_executors/step_executors/steps"
)

type StepNode struct {
	Step        steps.WorkflowStep                 `json:"step"`
	Transitions map[ExecutionState]*steps.StepName `json:"transitions"`
}
