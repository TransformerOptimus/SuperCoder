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

type DesignStoryReviewService struct {
	designStoryReviewRepo *repositories.DesignStoryReviewRepository
	asynqClient           *asynq.Client
	storyService          *StoryService
}

func NewDesignStoryReviewService(
	designStoryReviewRepo *repositories.DesignStoryReviewRepository, asynqClient *asynq.Client, storyService *StoryService,
) *DesignStoryReviewService {
	return &DesignStoryReviewService{
		designStoryReviewRepo: designStoryReviewRepo,
		asynqClient:           asynqClient,
		storyService:          storyService,
	}
}

func (s *DesignStoryReviewService) CreateComment(storyID uint, comment string) error {
	designStory := &models.DesignStoryReview{
		StoryID: storyID,
		Comment: comment,
	}
	err := s.designStoryReviewRepo.CreateDesignStoryReview(designStory)
	if err != nil {
		fmt.Println("Create design story review fail")
		return err
	}

	//Enqueue to Asynq
	payload := asynq_task.CreateJobPayload{
		StoryID:   storyID,
		ReExecute: true,
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

func (s *DesignStoryReviewService) GetAllDesignReviewsByStoryId(storyID uint) ([]*models.DesignStoryReview, error) {
	return s.designStoryReviewRepo.GetAllDesignReviewsByStoryId(storyID)
}
