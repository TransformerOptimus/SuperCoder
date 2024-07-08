package request

import (
	"os"
	"strings"
)

type CreateJobRequest struct {
	ExecutionId   int64    `json:"executionId"`
	ProjectId     string   `json:"projectId"`
	StoryId       int64    `json:"storyId"`
	IsReExecution bool     `json:"isReExecution"`
	Branch        string   `json:"branch"`
	PullRequestId int64    `json:"pullRequestId"`
	ExecutorImage string   `json:"executorImage"`
	Env           []string `json:"env"`
}

func (receiver *CreateJobRequest) WithStoryId(storyId int64) *CreateJobRequest {
	receiver.StoryId = storyId
	return receiver
}

func (receiver *CreateJobRequest) WithProjectId(projectId string) *CreateJobRequest {
	receiver.ProjectId = projectId
	return receiver
}

func (receiver *CreateJobRequest) WithIsReExecution(isReExecution bool) *CreateJobRequest {
	receiver.IsReExecution = isReExecution
	return receiver
}

func (receiver *CreateJobRequest) WithExecutorImage(executorImage string) *CreateJobRequest {
	receiver.ExecutorImage = executorImage
	return receiver
}

func (receiver *CreateJobRequest) WithBranch(branch string) *CreateJobRequest {
	receiver.Branch = branch
	return receiver
}

func (receiver *CreateJobRequest) WithPullRequestId(pullRequestId int64) *CreateJobRequest {
	receiver.PullRequestId = pullRequestId
	return receiver
}

func (receiver *CreateJobRequest) WithEnv(env []string) *CreateJobRequest {
	receiver.Env = env
	return receiver
}

func (receiver *CreateJobRequest) WithExecutionId(executionId int64) *CreateJobRequest {
	receiver.ExecutionId = executionId
	return receiver

}

func NewCreateJobRequest() *CreateJobRequest {
	envVars := make([]string, 0, len(os.Environ()))
	for _, envVar := range os.Environ() {
		if strings.HasPrefix(envVar, "AI_DEVELOPER_") {
			envVars = append(envVars, envVar)
		}
	}
	return &CreateJobRequest{
		Env: envVars,
	}
}

type CreateJobResponse struct {
	JobId string `json:"jobId"`
}
