package graph

import (
	"ai-developer/app/workflow_executors/step_executors/steps"
	"errors"
	"fmt"
)

type StepGraph struct {
	StartingNode steps.StepName               `json:"startingNode"`
	Nodes        map[steps.StepName]*StepNode `json:"nodes"`
}

func (g *StepGraph) GetStartingNode() steps.StepName {
	return g.StartingNode
}

func (g *StepGraph) GetNextStep(stepName steps.StepName, executionState ExecutionState) *steps.StepName {
	if stepNode, ok := g.Nodes[stepName]; ok {
		if nextStep, ok := stepNode.Transitions[executionState]; ok {
			return nextStep
		}
	}
	return nil
}

func (g *StepGraph) Walk(execute func(name steps.StepName, step steps.WorkflowStep) error) {
	currentStep := &g.StartingNode
	iteration := 0
	for currentStep != nil && iteration < 40 {
		err := execute(*currentStep, g.Nodes[*currentStep].Step)
		var executionState ExecutionState
		if err != nil {
			if errors.Is(err, steps.ErrReiterate) {
				fmt.Println("Retry Execution - Server Run Failed")
				executionState = ExecutionRetryState
			} else {
				executionState = ExecutionErrorState
			}
		} else {
			executionState = ExecutionSuccessState
		}
		currentStep = g.GetNextStep(*currentStep, executionState)
		iteration++
	}
	return
}
