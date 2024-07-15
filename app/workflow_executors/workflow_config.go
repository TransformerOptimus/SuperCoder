package workflow_executors

import (
	"ai-developer/app/workflow_executors/step_executors/graph"
)

type WorkflowConfig struct {
	WorkflowName string
	StepGraph    *graph.StepGraph
	Files        []string
}
