package impl

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
	"workspace-service/app/config"
	"workspace-service/app/models/dto"
	"workspace-service/app/services"
)

type DockerJobService struct {
	services.JobService
	dockerClient *client.Client
	logger       *zap.Logger
	config       *config.WorkspaceJobs
}

func getDockerEnvVars(request dto.CreateJobRequest) []string {
	executionEnvVars := []string{
		fmt.Sprintf("AI_DEVELOPER_EXECUTION_STORY_ID=%d", request.StoryId),
		fmt.Sprintf("AI_DEVELOPER_EXECUTION_REEXECUTION=%t", request.IsReExecution),
		fmt.Sprintf("AI_DEVELOPER_EXECUTION_BRANCH=%s", request.Branch),
		fmt.Sprintf("AI_DEVELOPER_EXECUTION_PULLREQUEST_ID=%d", request.PullRequestId),
		fmt.Sprintf("AI_DEVELOPER_EXECUTION_ID=%d", request.ExecutionId),
	}
	executionEnvVars = append(executionEnvVars, request.Env...)
	return executionEnvVars
}

func (js DockerJobService) CreateJob(request dto.CreateJobRequest) (res *dto.CreateJobResponse, err error) {
	jobName := createJobName(request.ProjectId, request.StoryId, request.ExecutionId)

	js.logger.Info(
		"Creating job", 
		zap.String("requestImage", request.ExecutorImage),
		zap.String("jobName", jobName), 
		zap.String("image", js.config.LocalContainerImage(request.ExecutorImage)),
	)

	cont, err := js.dockerClient.ContainerCreate(
		context.Background(),
		&container.Config{
			Image: js.config.LocalContainerImage(request.ExecutorImage),
			Env:   getDockerEnvVars(request),
		},
		&container.HostConfig{
			AutoRemove: js.config.AutoRemoveJobContainer(),
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: js.config.VolumeSource(),
					Target: js.config.VolumeTarget(),
				},
			},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				js.config.DockerNetwork(): {},
			},
		},
		nil,
		jobName,
	)
	if err != nil {
		js.logger.Sugar().Error("Failed to create docker container", zap.Error(err))
		return nil, err
	}
	err = js.dockerClient.ContainerStart(context.Background(), cont.ID, container.StartOptions{})
	if err != nil {
		return nil, err
	}
	res = &dto.CreateJobResponse{
		JobId: jobName,
	}
	return
}

func NewDockerJobService(
	dockerClient *client.Client,
	config *config.WorkspaceJobs,
	logger *zap.Logger,
) services.JobService {
	return &DockerJobService{
		dockerClient: dockerClient,
		logger:       logger.Named("DockerJobService"),
		config:       config,
	}
}
