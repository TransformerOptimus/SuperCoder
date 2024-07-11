package services

import (
	"ai-developer/app/client/workspace"
	"ai-developer/app/config"
	"ai-developer/app/constants"
	"ai-developer/app/models"
	"ai-developer/app/models/dtos/asynq_task"
	"ai-developer/app/models/types"
	"ai-developer/app/repositories"
	"ai-developer/app/services/s3_providers"
	"ai-developer/app/types/request"
	"ai-developer/app/types/response"
	"ai-developer/app/utils"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type StoryService struct {
	storyRepo              *repositories.StoryRepository
	storyTestCaseRepo      *repositories.StoryTestCaseRepository
	storyInstructionRepo   *repositories.StoryInstructionRepository
	asynqClient            *asynq.Client
	logger                 *zap.Logger
	s3Service              *s3_providers.S3Service
	storyFileRepo          *repositories.StoryFileRepository
	hashIdGenerator        *utils.HashIDGenerator
	workspaceServiceClient *workspace.WorkspaceServiceClient
	projectService         *ProjectService
}

func (s *StoryService) GetStoryById(storyId int64) (*models.Story, error) {
	return s.storyRepo.GetStoryById(int(storyId))
}

func (s *StoryService) CreateStoryForProject(requestData request.CreateStoryRequest) (int, error) {
	storyType := "backend"
	hashID := s.hashIdGenerator.Generate() + "-" + uuid.New().String()
	story := &models.Story{
		ProjectID:   uint(requestData.ProjectId),
		Title:       requestData.Summary,
		Description: requestData.Description,
		Status:      constants.Todo,
		HashID:      hashID,
		Type:        storyType,
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

func (s *StoryService) CreateDesignStoryForProject(file multipart.File, fileName, title string, projectID int, storyType string) (uint, error) {
	hashID := s.hashIdGenerator.Generate() + "-" + uuid.New().String()
	project, err := s.projectService.GetProjectById(uint(projectID))
	if err != nil {
		return 0, err
	}
	url := "http://localhost:8081/?folder=/workspaces/stories/" + hashID
	env := config.Get("app.env")
	frontendBaseUrl := config.WorkspaceStaticFrontendUrl()
	frontendUrl := frontendBaseUrl + "/stories/" + project.HashID + "/" + hashID + "/out/"
	if env == "production" {
		url = "https://" + hashID + ".workspace.superagi.com/?folder=/workspaces/stories/" + hashID
	}

	story := &models.Story{
		ProjectID:   uint(projectID),
		Title:       title,
		Status:      constants.Todo,
		HashID:      hashID,
		Url:         url,
		FrontendURL: frontendUrl,
		Type:        storyType,
	}

	frontendService := "nextjs"
	//Making Call to Workspace Service to create workspace on project level
	_, err = s.workspaceServiceClient.CreateFrontendWorkspace(
		&request.CreateWorkspaceRequest{
			StoryHashId:      hashID,
			WorkspaceId:      project.HashID,
			FrontendTemplate: &frontendService,
			//FrontendTemplate: &backendService,
		},
	)

	if err != nil {
		fmt.Println("Error creating workspace")
		return 0, err
	}

	// create a story
	createdStory, err := s.storyRepo.CreateStory(story)
	if err != nil {
		return 0, err
	}

	s3Url, err := s.UploadFileBytesToS3(file, fileName, projectID, int(createdStory.ID))
	if err != nil {
		fmt.Println("Error uploading file to S3", err.Error())
		err := s.DeleteStoryByID(int(createdStory.ID))
		if err!=nil{
			fmt.Println("Error deleting story")
		}
		return 0, err
	}

	//create a story file
	storyFile := &models.StoryFile{
		StoryID:  createdStory.ID,
		Name:     fileName,
		FilePath: s3Url,
	}

	err = s.storyFileRepo.CreateStoryFile(storyFile)
	if err != nil {
		fmt.Println("Error creating story file", err.Error())
		return 0, err
	}
	return createdStory.ID, nil
}

func (s *StoryService) UpdateDesignStory(file multipart.File, fileName, title string, storyID int) error {
	story, err := s.storyRepo.GetStoryById(storyID)
	if err != nil {
		fmt.Println("Error getting story by id", err.Error())
		return err
	}
	if story == nil {
		return errors.New("story not found")
	}
	err = s.storyRepo.UpdateStory(story, title, "")
	if err != nil {
		fmt.Println("Error updating story", err.Error())
		return err
	}
	if file == nil {
		return nil
	}
	storyFile, err := s.storyFileRepo.GetFileByStoryID(story.ID)
	if err != nil {
		fmt.Println("Error getting story file", err.Error())
		return err
	}
	err = s.s3Service.DeleteS3Object(storyFile.FilePath)
	if err != nil {
		fmt.Println("Error deleting story file", err.Error())
		return err
	}
	s3Url, err := s.UploadFileBytesToS3(file, fileName, int(story.ProjectID), storyID)
	if err != nil {
		fmt.Println("Error uploading file to S3", err.Error())
		return err
	}
	err = s.storyFileRepo.UpdateStoryFileUrl(storyFile, s3Url)
	if err != nil {
		fmt.Println("Error updating story file", err.Error())
		return err
	}
	return nil
}

func (s *StoryService) UploadFileBytesToS3(file multipart.File, fileName string, projectID, storyID int) (string, error) {
	//read file to bytes
	fileBytes, err := utils.ReadFileToBytes(file)
	if err != nil {
		fmt.Println("Error reading file", err.Error())
		return "", err
	}
	//upload image to s3
	s3Url, err := s.s3Service.UploadFileToS3(fileBytes, fileName, projectID, storyID)
	if err != nil {
		return "", err
	}
	return s3Url, err
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

func (s *StoryService) GetAllStoriesOfProject(projectId int, searchValue string, storyType string) (*response.GetAllStoriesByProjectIDResponse, error) {
	stories, err := s.storyRepo.GetStoriesByProjectIdAndSearch(projectId, searchValue, storyType)
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
		case constants.ExecutionEnqueued:
			storiesByStatus[constants.InProgress] = append(storiesByStatus[constants.InProgress], response.StoryData{
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

func (s *StoryService) GetDesignStoriesOfProject(projectId int, storyType string) ([]*response.GetDesignStoriesOfProjectId, error) {
	stories, err := s.storyRepo.GetStoriesByProjectId(projectId, storyType)
	fmt.Println(stories)
	if err != nil {
		return nil, err
	}
	var allDesignStories []*response.GetDesignStoriesOfProjectId
	for _, story := range stories {
		storyFile, err := s.storyFileRepo.GetFileByStoryID(story.ID)
		status := story.Status
		if status == constants.MaxLoopIterationReached {
			status = constants.InReview
		}
		if err != nil {

			return nil, err
		}
		allDesignStories = append(allDesignStories, &response.GetDesignStoriesOfProjectId{
			StoryID:           int(story.ID),
			StoryName:         story.Title,
			StoryStatus:       status,
			StoryInputFileURL: storyFile.FilePath,
			CreatedAt:         story.CreatedAt.Format("Jan 2"),
			ReviewViewed:      story.ReviewViewed,
			FrontendURL:       story.FrontendURL,
		})
	}
	return allDesignStories, nil
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

	storyFile, err := s.storyFileRepo.GetFileByStoryID(story.ID)
	if err != nil {
		storyDetailsResponse.StoryInputFileUrl = ""
	} else {
		storyDetailsResponse.StoryInputFileUrl = storyFile.FilePath
	}
	return storyDetailsResponse, nil

}

func (s *StoryService) GetCodeForDesignStory(storyId int) ([]*response.GetCodeForDesignStory, error) {
	var codeFiles []*response.GetCodeForDesignStory
	story, err := s.storyRepo.GetStoryById(storyId)
	if err != nil {
		return codeFiles, err
	}
	project, err := s.projectService.GetProjectById(story.ProjectID)
	if err != nil {
		return nil, err
	}
	folderPath := config.WorkspaceWorkingDirectory() + "/stories/" + project.HashID + "/" + story.HashID + "/app/"
	var fileData []*response.GetCodeForDesignStory
	files, err := os.ReadDir(folderPath)
	if err != nil {
		fmt.Printf("Error reading directory %s\n", folderPath)
		return nil, err
	}

	for _, file := range files {
		if !file.IsDir() && (strings.HasSuffix(file.Name(), ".css") || strings.HasSuffix(file.Name(), ".tsx")) {
			fullPath := filepath.Join(folderPath, file.Name())
			content, err := os.ReadFile(fullPath)
			if err != nil {
				fmt.Printf("Error reading file %s: %s\n", fullPath, err)
				return nil, err
			}
			fileData = append(fileData, &response.GetCodeForDesignStory{
				FileName: file.Name(),
				Code:     string(content),
			})
		}
	}
	return fileData, nil
}

func (s *StoryService) GetDesignStoryDetails(storyId int) (*response.GetDesignStoriesOfProjectId, error) {
	story, err := s.storyRepo.GetStoryById(storyId)
	if err != nil {
		return &response.GetDesignStoriesOfProjectId{}, err
	}
	if story.IsDeleted == true {
		return nil, nil
	}
	storyDetailsResponse := &response.GetDesignStoriesOfProjectId{
		StoryID:      storyId,
		StoryName:    story.Title,
		StoryStatus:  story.Status,
		CreatedAt:    story.CreatedAt.Format("Jan 2"),
		ReviewViewed: story.ReviewViewed,
		FrontendURL:  story.FrontendURL,
	}
	storyFile, err := s.storyFileRepo.GetFileByStoryID(story.ID)
	if err != nil {
		storyDetailsResponse.StoryInputFileURL = ""
	} else {
		storyDetailsResponse.StoryInputFileURL = storyFile.FilePath
	}
	return storyDetailsResponse, nil
}

func (s *StoryService) UpdateReviewViewed(storyId int, viewedStatus bool) error {
	story, err := s.storyRepo.GetStoryById(storyId)
	if err != nil {
		return err
	}
	err = s.storyRepo.UpdateReviewViewedStatus(story, viewedStatus)
	if err != nil {
		return err
	}
	return nil
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
			if err != nil {
				s.logger.Error("Error enqueuing task", zap.Error(err))
				return err
			}
			err = s.storyRepo.UpdateStoryStatus(story, constants.ExecutionEnqueued)
			if err != nil {
				s.logger.Error("Error updating story status", zap.Error(err))
				return err
			}

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

func (s *StoryService) GetStoriesByProjectId(projectID int) ([]models.Story, error) {
	storyType := "backend"
	stories, err := s.storyRepo.GetStoriesByProjectId(projectID, storyType)
	if err != nil {
		return nil, err
	}
	return stories, nil
}

func (s *StoryService) GetDesignStoriesByProjectId(projectID int) ([]models.Story, error) {
	storyType := "frontend"
	stories, err := s.storyRepo.GetStoriesByProjectId(projectID, storyType)
	if err != nil {
		return nil, err
	}
	return stories, nil
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

func (s *StoryService) GetStoryFileByStoryId(storyId uint) (*models.StoryFile, error) {
	storyFile, err := s.storyFileRepo.GetFileByStoryID(storyId)
	if err != nil {
		return nil, err
	}
	return storyFile, nil
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
	s3Service *s3_providers.S3Service,
	storyFileRepo *repositories.StoryFileRepository,
	workspaceServiceClient *workspace.WorkspaceServiceClient,
	projectService *ProjectService,
) *StoryService {
	return &StoryService{
		storyRepo:              storyRepo,
		storyTestCaseRepo:      storyTestCaseRepo,
		storyInstructionRepo:   storyInstructionRepo,
		asynqClient:            asynqClient,
		logger:                 logger.Named("StoryService"),
		s3Service:              s3Service,
		storyFileRepo:          storyFileRepo,
		hashIdGenerator:        utils.NewHashIDGenerator(5),
		workspaceServiceClient: workspaceServiceClient,
		projectService:         projectService,
	}
}
