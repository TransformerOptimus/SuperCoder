package services

import "workspace-service/app/models/dto"

type JobService interface {
	CreateJob(request dto.CreateJobRequest) (*dto.CreateJobResponse, error)
}
