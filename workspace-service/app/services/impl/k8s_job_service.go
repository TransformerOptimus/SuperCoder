package impl

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	v1 "k8s.io/api/batch/v1"
	v13 "k8s.io/api/core/v1"
	v14 "k8s.io/api/policy/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"strings"
	"workspace-service/app/config"
	"workspace-service/app/models/dto"
	"workspace-service/app/services"
)

type K8sJobService struct {
	services.JobService
	clientset              *kubernetes.Clientset
	jobsConfig             *config.WorkspaceJobs
	workspaceServiceConfig *config.WorkspaceServiceConfig
	logger                 *zap.Logger
}

func createJobName(
	projectHashId string,
	storyId int64,
	executionId int64,
) string {
	return fmt.Sprintf("job-%s-%d-%d", projectHashId, storyId, executionId)
}

func getKubernetesEnvVars(request dto.CreateJobRequest) []v13.EnvVar {
	executionEnvVars := []v13.EnvVar{
		{Name: "AI_DEVELOPER_EXECUTION_STORY_ID", Value: fmt.Sprintf("%d", request.StoryId)},
		{Name: "AI_DEVELOPER_EXECUTION_REEXECUTION", Value: fmt.Sprintf("%t", request.IsReExecution)},
		{Name: "AI_DEVELOPER_EXECUTION_BRANCH", Value: request.Branch},
		{Name: "AI_DEVELOPER_EXECUTION_PULLREQUEST_ID", Value: fmt.Sprintf("%d", request.PullRequestId)},
		{Name: "AI_DEVELOPER_EXECUTION_ID", Value: fmt.Sprintf("%d", request.ExecutionId)},
	}
	for _, env := range request.Env {
		envParts := strings.SplitN(env, "=", 2)
		if len(envParts) == 2 {
			executionEnvVars = append(
				executionEnvVars,
				v13.EnvVar{
					Name:  envParts[0],
					Value: envParts[1],
				},
			)
		}
	}
	return executionEnvVars
}

func (js K8sJobService) CreateJob(request dto.CreateJobRequest) (res *dto.CreateJobResponse, err error) {
	var ttlSecondsAfterFinished int32 = 86400 * 2
	jobName := createJobName(request.ProjectId, request.StoryId, request.ExecutionId)
	job := &v1.Job{
		ObjectMeta: v12.ObjectMeta{
			Name: jobName,
		},
		Spec: v1.JobSpec{
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: v13.PodTemplateSpec{
				ObjectMeta: v12.ObjectMeta{
					Labels: map[string]string{
						"app": jobName,
					},
				},
				Spec: v13.PodSpec{
					NodeSelector: map[string]string{
						"karpenter.sh/capacity-type": "on-demand",
					},
					RestartPolicy: v13.RestartPolicyNever,
					Containers: []v13.Container{
						{
							Name:            "execution",
							Image:           js.jobsConfig.ContainerImage(),
							ImagePullPolicy: v13.PullAlways,
							Env:             getKubernetesEnvVars(request),
							VolumeMounts: []v13.VolumeMount{
								{
									Name:      "workspace",
									MountPath: fmt.Sprintf("/workspaces/%s", request.ProjectId),
								},
							},
						},
					},
					Volumes: []v13.Volume{
						{
							Name: "workspace",
							VolumeSource: v13.VolumeSource{
								PersistentVolumeClaim: &v13.PersistentVolumeClaimVolumeSource{
									ClaimName: request.ProjectId,
								},
							},
						},
					},
				},
			},
		},
	}
	job, err = js.clientset.
		BatchV1().
		Jobs(js.workspaceServiceConfig.WorkspaceNamespace()).
		Create(context.Background(), job, v12.CreateOptions{})
	if err != nil {
		return nil, err
	}
	pdb := v14.PodDisruptionBudget{
		ObjectMeta: v12.ObjectMeta{
			Name: fmt.Sprintf("pdb-%s", jobName),
		},
		Spec: v14.PodDisruptionBudgetSpec{
			MinAvailable: &intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: 1,
			},
			Selector: &v12.LabelSelector{
				MatchLabels: map[string]string{
					"app": jobName,
				},
			},
		},
	}
	_, err = js.clientset.PolicyV1().PodDisruptionBudgets(js.workspaceServiceConfig.WorkspaceNamespace()).Create(context.Background(), &pdb, v12.CreateOptions{})
	if err != nil {
		js.logger.Error("Failed to create PodDisruptionBudget", zap.Error(err))
	}
	res = &dto.CreateJobResponse{
		JobId: job.Name,
	}
	return
}

func NewK8sJobService(
	clientset *kubernetes.Clientset,
	jobsConfig *config.WorkspaceJobs,
	workspaceServiceConfig *config.WorkspaceServiceConfig,
	logger *zap.Logger,
) services.JobService {
	return &K8sJobService{
		clientset:              clientset,
		jobsConfig:             jobsConfig,
		workspaceServiceConfig: workspaceServiceConfig,
		logger:                 logger.Named("K8sJobService"),
	}
}
