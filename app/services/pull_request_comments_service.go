package services

import (
	"ai-developer/app/constants"
	"ai-developer/app/models"
	"ai-developer/app/models/dtos/asynq_task"
	"ai-developer/app/repositories"
	"encoding/json"
	"fmt"
	"github.com/hibiken/asynq"
	"time"
)

type PullRequestCommentsService struct {
	pullRequestCommentsRepo *repositories.PullRequestCommentsRepository
	pullRequestRepo         *repositories.PullRequestRepository
	storyService            *StoryService
	asynqClient             *asynq.Client
}

func NewPullRequestCommentsService(
	pullRequestCommentsRepo *repositories.PullRequestCommentsRepository,
	pullRequestRepo *repositories.PullRequestRepository,
	storyService *StoryService,
	asynqClient *asynq.Client,
) *PullRequestCommentsService {
	return &PullRequestCommentsService{
		pullRequestCommentsRepo: pullRequestCommentsRepo,
		pullRequestRepo:         pullRequestRepo,
		storyService:            storyService,
		asynqClient:             asynqClient,
	}
}
func (s *PullRequestCommentsService) CreateComment(pullRequestID uint, comment string) error {
	err := s.pullRequestCommentsRepo.CreateComment(pullRequestID, comment)
	if err != nil {
		return err
	}
	fmt.Println("Enquing Comment to execute!")
	pullRequest, _ := s.pullRequestRepo.GetPullRequestByID(pullRequestID)

	//Enqueue to Asynq
	payload := asynq_task.CreateJobPayload{
		StoryID:       pullRequest.StoryID,
		ReExecute:     true,
		PullRequestId: pullRequestID,
	}
	// Serialize the payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// Create and enqueue the task
	task := asynq.NewTask(constants.CreateExecutionJobTaskType, payloadBytes)
	// Enqueue the task with exponential backoff
	_, err = s.asynqClient.Enqueue(task,
		asynq.Unique(30*time.Second),
		asynq.MaxRetry(5),
	)

	if err != nil {
		fmt.Println("Err : ", err)
		return err
	}

	return nil
}

func (s *PullRequestCommentsService) GetAllCommentsByPullRequestID(pullRequestID uint) ([]models.PullRequestComments, error) {
	return s.pullRequestCommentsRepo.GetAllCommentsByPullRequestID(pullRequestID)
}
