package workflow_executors

import (
	"ai-developer/app/workflow_executors/step_executors/graph"
	"ai-developer/app/workflow_executors/step_executors/steps"
)

var NextJsWorkflowConfig = &WorkflowConfig{
	WorkflowName: "Next JS Workflow",
	StepGraph: &graph.StepGraph{
		StartingNode: steps.CODE_GENERATE_CSS_STEP,
		Nodes: map[steps.StepName]*graph.StepNode{
			steps.CODE_GENERATE_CSS_STEP: {
				Step: &steps.GenerateCodeStep{
					MaxLoopIterations: 15,
					File:              "globals.css",
				},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.UPDATE_CODE_CSS_FILE_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.UPDATE_CODE_CSS_FILE_STEP: {
				Step: &steps.UpdateCodeFileStep{
					File: "globals.css",
				},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.CODE_GENERATE_PAGE_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.CODE_GENERATE_LAYOUT_STEP: {
				Step: &steps.GenerateCodeStep{
					MaxLoopIterations: 15,
					File:              "layout.tsx",
				},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.UPDATE_CODE_LAYOUT_FILE_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.UPDATE_CODE_LAYOUT_FILE_STEP: {
				Step: &steps.UpdateCodeFileStep{
					File: "layout.tsx",
				},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.SERVER_START_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.CODE_GENERATE_PAGE_STEP: {
				Step: &steps.GenerateCodeStep{
					MaxLoopIterations: 15,
					File:              "page.tsx",
				},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.UPDATE_CODE_PAGE_FILE_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.UPDATE_CODE_PAGE_FILE_STEP: {
				Step: &steps.UpdateCodeFileStep{
					File: "page.tsx",
				},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.CODE_GENERATE_LAYOUT_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.SERVER_START_STEP: {
				Step: &steps.ServerStartTestStep{},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: nil,
					graph.ExecutionRetryState:   &steps.RETRY_CODE_GENERATE_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.RETRY_CODE_GENERATE_STEP: {
				Step: &steps.GenerateCodeStep{
					MaxLoopIterations: 15,
					Retry:             true,
				},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.UPDATE_CODE_FILE_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.UPDATE_CODE_FILE_STEP: {
				Step: &steps.UpdateCodeFileStep{
					Retry: true,
				},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.SERVER_START_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
		},
	},
}

// generate code for file 1
// update code for file 1
// generate code for file 2
// update code for file 2
// generate
