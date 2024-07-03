package workflow_executors

import (
	"ai-developer/app/workflow_executors/step_executors/graph"
	"ai-developer/app/workflow_executors/step_executors/steps"
)

var FlaskWorkflowConfig = &WorkflowConfig{
	WorkflowName: "Flask Workflow",
	StepGraph: &graph.StepGraph{
		StartingNode: steps.GIT_CREATE_BRANCH_STEP,
		Nodes: map[steps.StepName]*graph.StepNode{
			steps.GIT_CREATE_BRANCH_STEP: {
				Step: &steps.GitMakeBranchStep{},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.RESET_DB_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.RESET_DB_STEP: {
				Step: &steps.ResetDBStep{},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.CODE_GENERATE_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.CODE_GENERATE_STEP: {
				Step: &steps.GenerateCodeStep{
					MaxLoopIterations: 10,
				},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.UPDATE_CODE_FILE_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},
			steps.RETRY_CODE_GENERATE_STEP: {
				Step: &steps.GenerateCodeStep{
					MaxLoopIterations: 10,
					Retry:             true,
				},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.UPDATE_CODE_FILE_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},

			steps.UPDATE_CODE_FILE_STEP: {
				Step: &steps.UpdateCodeFileStep{},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.SERVER_START_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},

			steps.SERVER_START_STEP: {
				Step: &steps.ServerStartTestStep{},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.GIT_COMMIT_STEP,
					graph.ExecutionRetryState:   &steps.RETRY_CODE_GENERATE_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},

			steps.GIT_COMMIT_STEP: {
				Step: &steps.GitCommitStep{},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.GIT_PUSH_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},

			steps.GIT_PUSH_STEP: {
				Step: &steps.GitPushStep{},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: &steps.GIT_CREATE_PULL_REQUEST_STEP,
					graph.ExecutionErrorState:   nil,
				},
			},

			steps.GIT_CREATE_PULL_REQUEST_STEP: {
				Step: &steps.GitMakePullRequestStep{},
				Transitions: map[graph.ExecutionState]*steps.StepName{
					graph.ExecutionSuccessState: nil,
					graph.ExecutionErrorState:   nil,
				},
			},
		},
	},
}
