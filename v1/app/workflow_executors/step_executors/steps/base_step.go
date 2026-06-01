package steps

import (
	"ai-developer/app/models"
	"errors"
)

var ErrReiterate = errors.New("reiterate error")

type BaseStep struct {
	Story         *models.Story
	Project       *models.Project
	ExecutionStep *models.ExecutionStep
	Execution     *models.Execution
}

func (s *BaseStep) WithProject(project *models.Project) *BaseStep {
	s.Project = project
	return s
}

func (s *BaseStep) WithStory(story *models.Story) *BaseStep {
	s.Story = story
	return s
}

func (s *BaseStep) WithExecutionStep(executionStep *models.ExecutionStep) *BaseStep {
	s.ExecutionStep = executionStep
	return s
}

func (s *BaseStep) WithExecution(execution *models.Execution) *BaseStep {
	s.Execution = execution
	return s
}
