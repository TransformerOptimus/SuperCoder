package services

import (
	"ai-developer/app/constants"
	"ai-developer/app/models"
	"ai-developer/app/models/dtos/asynq_task"
	"ai-developer/app/models/types"
	"ai-developer/app/repositories"
	"ai-developer/app/types/request"
	"ai-developer/app/types/response"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"log"
	"sort"
	"strings"
)

type StoryService struct {
	storyRepo            *repositories.StoryRepository
	storyTestCaseRepo    *repositories.StoryTestCaseRepository
	storyInstructionRepo *repositories.StoryInstructionRepository
	asynqClient          *asynq.Client
	logger               *zap.Logger
}

func (s *StoryService) GetStoryById(storyId int64) (*models.Story, error) {
	return s.storyRepo.GetStoryById(int(storyId))
}

func (s *StoryService) CreateStoryForProject(requestData request.CreateStoryRequest) (int, error) {
	story := &models.Story{
		ProjectID:   uint(requestData.ProjectId),
		Title:       requestData.Summary,
		Description: requestData.Description,
		Status:      constants.Todo,
	}

	// create a story
	createdStory, err := s.storyRepo.CreateStory(story)
	if err != nil {
		return 0, err
	}

	// create story test cases
	for _, testCase := range requestData.TestCases {
		storyTestCase := &models.StoryTestCase{
			StoryID:  createdStory.ID, // Use the same story ID for all test cases
			TestCase: testCase,
		}
		// Insert the storyTestCase record
		if err := s.storyTestCaseRepo.CreateStoryTestCase(storyTestCase); err != nil {
			return 0, err
		}
	}

	// create story instructions
	storyInstruction := &models.StoryInstruction{
		StoryID:     createdStory.ID, // Use the same story ID for all test cases
		Instruction: requestData.Instructions,
	}
	// Insert the storyInstruction record
	if err := s.storyInstructionRepo.CreateStoryInstructions(storyInstruction); err != nil {
		return 0, err
	}
	return int(createdStory.ID), nil
}

func (s *StoryService) UpdateStoryForProject(requestData request.UpdateStoryRequest) error {
	story, err := s.storyRepo.GetStoryById(requestData.StoryID)
	if err != nil {
		fmt.Println("Error fetching story", err.Error())
		return err
	}
	if story == nil {
		fmt.Println("Story not found")
		return errors.New("Story not found")
	}
	err = s.storyRepo.UpdateStory(story, requestData.Summary, requestData.Description)
	if err != nil {
		return err
	}
	err = s.UpdateStoryTestCases(requestData.TestCases, requestData.StoryID)
	if err != nil {
		return err
	}
	storyInstruction, err := s.storyInstructionRepo.GetOneStoryInstructionByStoryId(requestData.StoryID)
	err = s.storyInstructionRepo.UpdateStoryInstructions(storyInstruction, requestData.Instructions)
	if err != nil {
		fmt.Println("Error updating story instructions", err.Error())
		return err
	}
	return nil
}

func (s *StoryService) UpdateStoryTestCases(newTestCases []string, storyID int) error {
	existingTestCases, err := s.storyTestCaseRepo.GetStoryTestCaseByStoryId(storyID)
	if err != nil {
		fmt.Println("Error fetching story test cases", err.Error())
		return err
	}

	existingTestCaseMap := make(map[string]uint)
	for _, testCase := range existingTestCases {
		existingTestCaseMap[testCase.TestCase] = testCase.ID
	}

	newTestCaseMap := make(map[string]bool)
	for _, testCase := range newTestCases {
		newTestCaseMap[testCase] = true
	}

	for _, testCase := range newTestCases {
		if _, exists := existingTestCaseMap[testCase]; !exists {
			storyTestCase := &models.StoryTestCase{
				StoryID:  uint(storyID), // Use the same story ID for all test cases
				TestCase: testCase,
			}
			if err = s.storyTestCaseRepo.CreateStoryTestCase(storyTestCase); err != nil {
				return err
			}
		}
	}

	for testCase, id := range existingTestCaseMap {
		if _, exists := newTestCaseMap[testCase]; !exists {
			storyTestCase, err1 := s.storyTestCaseRepo.GetStoryTestCaseById(int(id))
			if err1 != nil {
				log.Fatal(err1)
			}
			err = s.storyTestCaseRepo.DeleteStoryTestCaseByID(storyTestCase)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *StoryService) GetAllStoriesOfProject(projectId int, searchValue string) (*response.GetAllStoriesByProjectIDResponse, error) {
	stories, err := s.storyRepo.GetStoriesByProjectIdAndSearch(projectId, searchValue)
	if err != nil {
		return &response.GetAllStoriesByProjectIDResponse{}, err
	}
	storiesByStatus := map[string][]response.StoryData{
		constants.Todo:       {},
		constants.InProgress: {},
		constants.Done:       {},
		constants.InReview:   {},
	}
	// Sort stories by created_at in descending order
	sort.SliceStable(stories, func(i, j int) bool {
		return stories[i].CreatedAt.After(stories[j].CreatedAt)
	})
	// Iterate over stories and organize them by status
	for _, story := range stories {
		switch story.Status {
		case constants.Todo:
			storiesByStatus[constants.Todo] = append(storiesByStatus[constants.Todo], response.StoryData{
				StoryID:   int(story.ID),
				StoryName: story.Title,
			})
		case constants.InProgress:
			storiesByStatus[constants.InProgress] = append(storiesByStatus[constants.InProgress], response.StoryData{
				StoryID:   int(story.ID),
				StoryName: story.Title,
			})
		case constants.Done:
			storiesByStatus[constants.Done] = append(storiesByStatus[constants.Done], response.StoryData{
				StoryID:   int(story.ID),
				StoryName: story.Title,
			})
		default:
			storiesByStatus[constants.InReview] = append(storiesByStatus[constants.InReview], response.StoryData{
				StoryID:   int(story.ID),
				StoryName: story.Title,
			})
		}
	}

	// Create response format
	responseData := &response.GetAllStoriesByProjectIDResponse{
		Todo:       storiesByStatus[constants.Todo],
		InProgress: storiesByStatus[constants.InProgress],
		Done:       storiesByStatus[constants.Done],
		InReview:   storiesByStatus[constants.InReview],
	}

	return responseData, nil
}

func (s *StoryService) GetStoryDetails(storyId int) (*response.GetStoryByIdResponse, error) {
	story, err := s.storyRepo.GetStoryById(storyId)
	if err != nil {
		return &response.GetStoryByIdResponse{}, err
	}
	if story.IsDeleted == true {
		return nil, nil
	}
	storyTestCases, err := s.storyTestCaseRepo.GetStoryTestCaseByStoryId(storyId)
	if err != nil {
		return &response.GetStoryByIdResponse{}, err
	}
	storyInstructions, err := s.storyInstructionRepo.GetStoryInstructionByStoryId(storyId)
	if err != nil {
		return &response.GetStoryByIdResponse{}, err
	}
	storyDetailsResponse := &response.GetStoryByIdResponse{
		Overview: response.StoryOverview{
			Name:        story.Title,
			Description: story.Description,
		},
		TestCases:    make([]string, len(storyTestCases)),
		Instructions: make([]string, len(storyInstructions)),
		Status:       story.Status,
	}
	for i, testCase := range storyTestCases {
		storyDetailsResponse.TestCases[i] = testCase.TestCase
	}

	// Populate instructions in the response
	for i, instruction := range storyInstructions {
		storyDetailsResponse.Instructions[i] = instruction.Instruction
	}

	// If story status is not TODO, IN_PROGRESS or DONE then update it to IN_REVIEW
	storyStatusSet := map[string]struct{}{constants.Todo: {}, constants.InProgress: {}, constants.Done: {}}
	if _, found := storyStatusSet[story.Status]; found {
		storyDetailsResponse.Reason = ""
	} else {
		storyDetailsResponse.Reason = story.Status
		storyDetailsResponse.Status = constants.InReview
	}

	return storyDetailsResponse, nil
}

func (s *StoryService) GetInProgressStoriesByProjectId(projectId int) ([]*response.GetStoryResponse, error) {
	stories, err := s.storyRepo.GetInProgressStoriesByProjectId(projectId)
	if err != nil {
		return nil, err
	}
	var allInProgressStories []*response.GetStoryResponse
	for _, story := range stories {
		allInProgressStories = append(allInProgressStories, &response.GetStoryResponse{
			StoryId:    int(story.ID),
			StoryTitle: story.Title,
		})
	}
	return allInProgressStories, nil
}

func (s *StoryService) UpdateStoryStatusByUser(storyID int, status string) error {
	s.logger.Info("Updating story status by user", zap.Int("storyID", storyID), zap.String("status", status))
	story, err := s.GetStoryById(int64(storyID))
	if err != nil {
		s.logger.Error("Error fetching story", zap.Error(err))
		return types.ErrInvalidStory
	}
	if story.IsDeleted == true {
		s.logger.Info("Story is deleted", zap.Int("storyID", storyID))
		return types.ErrStoryDeleted
	}

	if !constants.ValidStatuses()[status] {
		s.logger.Error("Invalid status", zap.String("status", status))
		return types.ErrInvalidStatus
	}

	//Check if valid transition
	if status == constants.InProgress {
		if story.Status == constants.Todo || story.Status == constants.InReview {
			err := s.UpdateStoryStatus(storyID, status)
			if err != nil {
				s.logger.Error("Error updating story status", zap.Error(err))
				return err
			}
		}
	} else {
		return types.ErrInvalidStoryStatusTransition
	}

	return nil
}

func (s *StoryService) UpdateStoryStatus(storyID int, status string) error {
	s.logger.Info("Updating story status", zap.Int("storyID", storyID), zap.String("status", status))
	story, err := s.GetStoryById(int64(storyID))
	if err != nil {
		s.logger.Error("Error fetching story", zap.Error(err))
		return err
	}
	if story.IsDeleted == true {
		s.logger.Info("Story is deleted", zap.Int("storyID", storyID))
		return nil
	}

	if !constants.ValidStatuses()[status] {
		s.logger.Error("Invalid status", zap.String("status", status))
		return types.ErrInvalidStatus
	}

	s.logger.Info("Current Status", zap.String("status", story.Status))
	s.logger.Info("New Status", zap.String("status", status))
	if strings.ToUpper(status) == constants.InProgress {
		s.logger.Info("Story to be updated to InProgress", zap.Int("storyID", storyID))
		if story.Status == constants.Todo || story.Status == constants.InReview {
			s.logger.Info("Story is in Todo", zap.Int("storyID", storyID))
			s.logger.Info("Executing story", zap.Int("storyID", storyID))
			// Create payload for CreateJob task
			payload := asynq_task.CreateJobPayload{
				StoryID:       story.ID,
				ReExecute:     false,
				PullRequestId: 0,
			}
			s.logger.Info("Payload for creation job", zap.Any("payload", payload))
			// Serialize the payload to JSON
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				return err
			}

			// Create and enqueue the task
			task := asynq.NewTask(constants.CreateExecutionJobTaskType, payloadBytes)

			// Enqueue the task with exponential backoff
			_, err = s.asynqClient.Enqueue(task, asynq.MaxRetry(5))

		} else {
			s.logger.Info("Story already in progress", zap.Int("storyID", storyID))
			return errors.New("story already in progress")
		}
	} else {
		s.logger.Info("Story to be updating to", zap.String("status", status))
		err = s.storyRepo.UpdateStoryStatus(story, status)
		if err != nil {
			s.logger.Error("Error updating story status", zap.Error(err))
			return err
		}
	}

	return nil
}

func (s *StoryService) DeleteStoryByID(storyId int) error {
	story, err := s.storyRepo.GetStoryById(storyId)
	if err != nil {
		return err
	}
	if story == nil {
		fmt.Println("Story does not exist")
		return err
	}
	err = s.storyRepo.DeleteStoryById(story)
	if err != nil {
		return err
	}
	return nil
}

func (s *StoryService) GetStoryInstructionByStoryId(storyId int) ([]models.StoryInstruction, error) {
	return s.storyInstructionRepo.GetStoryInstructionByStoryId(storyId)
}

func (s *StoryService) GetStoryTestCaseByStoryId(storyId int) ([]models.StoryTestCase, error) {
	return s.storyTestCaseRepo.GetStoryTestCaseByStoryId(storyId)
}
func (s *StoryService) GetStoryByExecutionID(executionID uint) (*models.Story, error) {
	return s.storyRepo.GetStoryByExecutionID(executionID)
}

func (s *StoryService) GetStoryByProjectIdAndStatus(projectId int, status string) (*models.Story, error) {
	return s.storyRepo.GetStoryByProjectIdAndStatus(projectId, status)

}

func (s *StoryService) UpdateStoryStatusWithTx(tx *gorm.DB, storyId int, progress string) error {
	return s.storyRepo.UpdateStoryStatusWithTx(tx, storyId, progress)
}

func NewStoryService(
	storyRepo *repositories.StoryRepository,
	storyTestCaseRepo *repositories.StoryTestCaseRepository,
	storyInstructionRepo *repositories.StoryInstructionRepository,
	asynqClient *asynq.Client,
	logger *zap.Logger,
) *StoryService {
	return &StoryService{
		storyRepo:            storyRepo,
		storyTestCaseRepo:    storyTestCaseRepo,
		storyInstructionRepo: storyInstructionRepo,
		asynqClient:          asynqClient,
		logger:               logger.Named("StoryService"),
	}
}
