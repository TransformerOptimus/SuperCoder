package services

import (
	"ai-developer/app/client/workspace"
	"ai-developer/app/config"
	"ai-developer/app/constants"
	"ai-developer/app/models"
	"ai-developer/app/models/dtos/asynq_task"
	"ai-developer/app/repositories"
	"ai-developer/app/services/git_providers"
	"ai-developer/app/types/request"
	"ai-developer/app/types/response"
	"ai-developer/app/utils"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"go.uber.org/zap"
	"strconv"
	"time"
)

type ProjectService struct {
	redisRepo              *repositories.ProjectConnectionsRepository
	projectRepo            *repositories.ProjectRepository
	organisationRepository *repositories.OrganisationRepository
	storyRepository        *repositories.StoryRepository
	pullRequestRepository  *repositories.PullRequestRepository
	gitnessService         *git_providers.GitnessService
	hashIdGenerator        *utils.HashIDGenerator
	workspaceServiceClient *workspace.WorkspaceServiceClient
	asynqClient            *asynq.Client
	logger                 *zap.Logger
}

func (s *ProjectService) GetAllProjectsOfOrganisation(organisationId int) ([]response.GetAllProjectsResponse, error) {
	projects, err := s.projectRepo.GetAllProjectsByOrganisationId(organisationId)
	if err != nil {
		return nil, err
	}
	projectsIds := make([]int, len(projects))
	for i, project := range projects {
		projectsIds[i] = int(project.ID)
	}
	projectStoryMap, err := s.storyRepository.GetStoryIdsMapByProjectIds(projectsIds)
	if err != nil {
		return nil, err
	}
	projectPullRequestMap, err := s.pullRequestRepository.GetPullRequestsIdsByProjectAndStatus(projectStoryMap, constants.Open)
	if err != nil {
		return nil, err
	}
	allProjects := make([]response.GetAllProjectsResponse, 0, len(projects))
	for _, project := range projects {
		allProjects = append(allProjects, response.GetAllProjectsResponse{
			ProjectId:          project.ID,
			ProjectName:        project.Name,
			ProjectDescription: project.Description,
			ProjectFramework:   project.Framework,
			ProjectFrontendFramework: project.FrontendFramework,
			ProjectHashID:      project.HashID,
			ProjectUrl:         project.Url,
			ProjectBackendURL:  project.BackendURL,
			ProjectFrontendURL: project.FrontendURL,
			PullRequestCount:   len(projectPullRequestMap[int(project.ID)]),
		})
	}

	if allProjects == nil {
		allProjects = []response.GetAllProjectsResponse{}
	}

	return allProjects, nil
}

func (s *ProjectService) GetProjectDetailsById(projectId int) (*models.Project, error) {
	project, err := s.projectRepo.GetProjectById(projectId)
	if err != nil {
		return nil, err
	}
	return project, nil
}

func (s *ProjectService) CreateProject(organisationID int, requestData request.CreateProjectRequest) (*models.Project, error) {
	hashID := s.hashIdGenerator.Generate() + "-" + uuid.New().String()
	url := "http://localhost:8081/?folder=/workspaces/" + hashID
	backend_url := "http://localhost:5000"
	frontend_url := "http://localhost:3000"
	env := config.Get("app.env")
	host := config.Get("workspace.host")
	if env == "production" {
		url = fmt.Sprintf("https://%s.%s/?folder=/workspaces/%s", hashID, host, hashID)
		backend_url = fmt.Sprintf("https://be-%s.%s", hashID, host)
		frontend_url = fmt.Sprintf("https://fe-%s.%s", hashID, host)
	}
	project := &models.Project{
		OrganisationID:    uint(organisationID),
		Name:              requestData.Name,
		Framework:         requestData.Framework,
		FrontendFramework: requestData.FrontendFramework,
		Description:       requestData.Description,
		HashID:            hashID,
		Url:               url,
		BackendURL:        backend_url,
		FrontendURL:       frontend_url,
	}

	organisation, err := s.organisationRepository.GetOrganisationByID(uint(int(project.OrganisationID)))
	spaceOrProjectName := s.gitnessService.GetSpaceOrProjectName(organisation)
	repository, err := s.gitnessService.CreateRepository(spaceOrProjectName, project.Name, project.Description)
	if err != nil {
		s.logger.Error("Error creating repository", zap.Error(err))
		return nil, err
	}
	httpPrefix := "https"

	if config.AppEnv() == constants.Development {
		httpPrefix = "http"
	}

	remoteGitURL := fmt.Sprintf("%s://%s:%s@%s/git/%s/%s.git", httpPrefix, config.GitnessUser(), config.GitnessToken(), config.GitnessHost(), spaceOrProjectName, project.Name)
	backendService := requestData.Framework
	frontendService := requestData.FrontendFramework
	//Making Call to Workspace Service to create workspace on project level
	_, err = s.workspaceServiceClient.CreateWorkspace(
		&request.CreateWorkspaceRequest{
			WorkspaceId:     hashID,
			BackendTemplate: &backendService,
			FrontendTemplate: &frontendService,
			RemoteURL:       remoteGitURL,
			GitnessUserName: config.GitnessUser(),
			GitnessToken:    config.GitnessToken(),
		},
	)

	if err != nil {
		s.logger.Error("Error creating workspace", zap.Error(err))
		return nil, err
	}

	//Enqueue job to delete workspace with updated delay
	payloadBytes, err := json.Marshal(asynq_task.CreateDeleteWorkspaceTaskPayload{
		WorkspaceID: project.HashID,
	})
	if err != nil {
		s.logger.Error("Failed to marshal payload", zap.Error(err))
		return nil, err
	}
	_, err = s.asynqClient.Enqueue(
		asynq.NewTask(constants.DeleteWorkspaceTaskType, payloadBytes),
		asynq.ProcessIn(constants.ProjectConnectionTTL+10*time.Minute),
		asynq.MaxRetry(3),
		asynq.TaskID("delete:fallback:"+project.HashID),
	)

	s.logger.Info("Project created successfully with repository", zap.Any("project", project), zap.Any("repository", repository))
	return s.projectRepo.CreateProject(project)
}
func (s *ProjectService) CreateProjectWorkspace(projectID int, backendTemplate string) error {
	project, err := s.projectRepo.GetProjectById(projectID)
	if err != nil {
		return err
	}

	//Check if there is any active workspace
	currentActiveCount, err := s.GetActiveProjectCount(project.HashID)
	s.logger.Info("Initially Active Count", zap.Int("active_count", currentActiveCount))
	if err != nil {
		s.logger.Error("Failed to get active project count", zap.Error(err))
		return err
	}

	organisation, err := s.organisationRepository.GetOrganisationByID(uint(int(project.OrganisationID)))
	spaceOrProjectName := s.gitnessService.GetSpaceOrProjectName(organisation)
	httpPrefix := "https"

	if config.AppEnv() == constants.Development {
		httpPrefix = "http"
	}
	remoteGitURL := fmt.Sprintf("%s://%s:%s@%s/git/%s/%s.git", httpPrefix, config.GitnessUser(), config.GitnessToken(), config.GitnessHost(), spaceOrProjectName, project.Name)
	s.logger.Info("Active count is less than 1, creating workspace....")
	_, err = s.workspaceServiceClient.CreateWorkspace(
		&request.CreateWorkspaceRequest{
			WorkspaceId:     project.HashID,
			BackendTemplate: &backendTemplate,
			//FrontendTemplate: &backendService,
			RemoteURL:       remoteGitURL,
			GitnessUserName: config.GitnessUser(),
			GitnessToken:    config.GitnessToken(),
		})
	if err != nil {
		s.logger.Error("Failed to create workspace", zap.Error(err))
		return err
	}

	//Increment active project count
	_, err = s.redisRepo.IncrementActiveCount(project.HashID, constants.ProjectConnectionTTL)
	if err != nil {
		s.logger.Error("Failed to set active project count", zap.Error(err))
		return err
	}

	//Enqueue a schedule job on creation to delete workspace after 6 hours
	payloadBytes, err := json.Marshal(asynq_task.CreateDeleteWorkspaceTaskPayload{
		WorkspaceID: project.HashID,
	})
	if err != nil {
		s.logger.Error("Failed to marshal payload", zap.Error(err))
		return err
	}
	_, err = s.asynqClient.Enqueue(
		asynq.NewTask(constants.DeleteWorkspaceTaskType, payloadBytes),
		asynq.ProcessIn(constants.ProjectConnectionTTL+10*time.Minute),
		asynq.MaxRetry(3),
		asynq.TaskID("delete:fallback:"+project.HashID),
	)
	return nil
}

func (s *ProjectService) DeleteProjectWorkspace(projectID int) error {
	project, err := s.projectRepo.GetProjectById(projectID)
	if err != nil {
		return err
	}
	//Check if there is any active workspace
	currentActiveCount, err := s.GetActiveProjectCount(project.HashID)
	s.logger.Info("Initially Active Count", zap.Int("active_count", currentActiveCount))
	if err != nil {
		s.logger.Error("Failed to get active project count", zap.Error(err))
		return err
	}
	//If no active workspace, delete the workspace
	if currentActiveCount-1 < 1 {
		s.logger.Info("Active count becoming less than 1, deleting workspace....")
		//Handle Workspace Shutdown with asynq job
		payload := asynq_task.CreateDeleteWorkspaceTaskPayload{
			WorkspaceID: project.HashID,
		}
		s.logger.Info("Payload for creation job", zap.Any("payload", payload))
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			s.logger.Error("Failed to marshal payload", zap.Error(err))
			return err
		}
		task := asynq.NewTask(constants.DeleteWorkspaceTaskType, payloadBytes)
		_, err = s.asynqClient.Enqueue(task, asynq.ProcessIn(10*time.Minute), asynq.MaxRetry(3))
		if err != nil {
			s.logger.Error("Failed to enqueue delete workspace task", zap.Error(err))
			return err
		}
	}
	//Decrement active project count
	_, err = s.redisRepo.DecrementActiveCount(project.HashID, constants.ProjectConnectionTTL)
	return nil
}

func (s *ProjectService) UpdateProject(requestData request.UpdateProjectRequest) (*models.Project, error) {
	project, err := s.projectRepo.GetProjectById(requestData.ProjectID)
	if err != nil {
		return nil, err
	}
	updatedProject, err := s.projectRepo.UpdateProject(project, requestData)
	if err != nil {
		return nil, err
	}
	return updatedProject, nil
}

func (s *ProjectService) GetActiveProjectCount(workspaceID string) (int, error) {
	data, err := s.redisRepo.GetProjectData(workspaceID)
	if err != nil {
		s.logger.Error("Failed to get project data", zap.Error(err))
		return 0, err
	}
	s.logger.Info("Project Data from Redis", zap.Any("data", data))
	activeCountStr, ok := data["active_count"]
	if !ok {
		s.logger.Info("Active count not found in project data assuming 0")
		activeCountStr = "0"
		return 0, nil
	}
	activeCount, err := strconv.Atoi(activeCountStr)
	if err != nil {
		s.logger.Error("Failed to convert active_count to int", zap.Error(err))
		return 0, err
	}
	return activeCount, nil
}

func (s *ProjectService) GetMainBranchCommits(organisation *models.Organisation, projectName string) (int, string, error) {
	commits, err := s.gitnessService.GetAllCommitsOfProjectBranch(organisation, projectName)
	if err != nil {
		return 0, "", err
	}
	var lastCommitDate string
	if len(commits.Commits) > 0 {
		committerWhen := commits.Commits[0].Committer.When
		fmt.Println("Committer 'When':", committerWhen)
		lastCommitDate = utils.TimeAgo(commits.Commits[0].Committer.When, time.Now().UTC())
	} else {
		fmt.Println("No commits found.")
	}
	return commits.TotalCommits, lastCommitDate, nil
}

func (s *ProjectService) GetProjectById(projectId uint) (*models.Project, error) {
	return s.projectRepo.GetProjectById(int(projectId))
}

func NewProjectService(projectRepo *repositories.ProjectRepository,
	gitnessService *git_providers.GitnessService,
	organisationRepository *repositories.OrganisationRepository,
	storyRepository *repositories.StoryRepository,
	pullRequestRepository *repositories.PullRequestRepository,
	workspaceServiceClient *workspace.WorkspaceServiceClient,
	repo *repositories.ProjectConnectionsRepository,
	asynqClient *asynq.Client,
	logger *zap.Logger,
) *ProjectService {
	return &ProjectService{
		projectRepo:            projectRepo,
		gitnessService:         gitnessService,
		organisationRepository: organisationRepository,
		storyRepository:        storyRepository,
		pullRequestRepository:  pullRequestRepository,
		workspaceServiceClient: workspaceServiceClient,
		redisRepo:              repo,
		hashIdGenerator:        utils.NewHashIDGenerator(5),
		logger:                 logger.Named("ProjectService"),
		asynqClient:            asynqClient,
	}
}

func (s *PullRequestService) GetPullRequestWithDetails(pullRequestID uint) (*models.Project, error) {
	return s.pullRequestRepo.GetPullRequestWithDetails(pullRequestID)
}
