package services

import (
	"ai-developer/app/constants"
	"ai-developer/app/models"
	"ai-developer/app/models/dtos/gitness"
	"ai-developer/app/repositories"
	"ai-developer/app/services/git_providers"
	"ai-developer/app/types/response"
	"ai-developer/app/utils"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type PullRequestService struct {
	pullRequestRepo         *repositories.PullRequestRepository
	pullRequestCommentsRepo *repositories.PullRequestCommentsRepository
	gitService              *git_providers.GitnessService
	organisationRepo        *repositories.OrganisationRepository
	storyRepo               *repositories.StoryRepository
	projectRepo             *repositories.ProjectRepository
	executionRepo           *repositories.ExecutionRepository
	executionOutputRepo     *repositories.ExecutionOutputRepository
}

func NewPullRequestService(pullRequestRepo *repositories.PullRequestRepository, pullRequestCommentsRepo *repositories.PullRequestCommentsRepository,
	gitService *git_providers.GitnessService, organisationRepo *repositories.OrganisationRepository, storyRepo *repositories.StoryRepository,
	projectRepo *repositories.ProjectRepository, executionRepo *repositories.ExecutionRepository, executionOutputRepo *repositories.ExecutionOutputRepository) *PullRequestService {
	return &PullRequestService{
		pullRequestRepo:         pullRequestRepo,
		pullRequestCommentsRepo: pullRequestCommentsRepo,
		gitService:              gitService,
		organisationRepo:        organisationRepo,
		storyRepo:               storyRepo,
		projectRepo:             projectRepo,
		executionRepo:           executionRepo,
		executionOutputRepo:     executionOutputRepo,
	}
}

func (s *PullRequestService) GetAllPullRequests(projectID int, status string) ([]*response.GetAllPullRequests, error) {
	stories, err := s.storyRepo.GetStoriesByProjectId(projectID)
	if err != nil {
		return nil, err
	}
	storyIDs := make([]uint, len(stories))
	for i, story := range stories {
		storyIDs[i] = story.ID
	}
	pullRequests, err := s.pullRequestRepo.GetAllPullRequestsByStoryIDs(storyIDs, status)
	var allPullRequests []*response.GetAllPullRequests
	if err != nil {
		return nil, err
	}

	for _, pullRequest := range pullRequests {
		totalComments, err := s.GetTotalComments(pullRequest.ID)
		if err != nil {
			return nil, err
		}
		allPullRequests = append(allPullRequests, &response.GetAllPullRequests{
			PullRequestID:          int(pullRequest.ID),
			PullRequestDescription: pullRequest.PullRequestDescription,
			PullRequestNumber:      pullRequest.PullRequestNumber,
			PullRequestName:        pullRequest.PullRequestTitle,
			Status:                 pullRequest.Status,
			CreatedOn:              pullRequest.CreatedAt.Format("Jan 2"),
			MergedOn:               s.GetMergeDate(pullRequest.MergedAt, pullRequest.Status),
			ClosedOn:               s.GetClosedDate(pullRequest.ClosedAt, pullRequest.Status),
			TotalComments:          totalComments,
		})
	}

	return allPullRequests, nil
}

func (s *PullRequestService) MergePullRequestByID(pullRequestID int, organisationID uint) (*gitness.MergePullRequestResponse, error) {
	fmt.Println("Organisation ID: ", organisationID)
	fmt.Println("Pull Request ID: ", pullRequestID)
	organisation, err := s.organisationRepo.GetOrganisationByID(organisationID)
	pullRequest, err := s.pullRequestRepo.GetPullRequestByID(uint(pullRequestID))
	fmt.Println("Organisation: ", organisation)
	fmt.Println("Pull Request: ", pullRequest)
	if err != nil {
		fmt.Println("Error fetching Pull Request by ID")
		return nil, err
	}
	if pullRequest == nil {
		fmt.Println("Pull Request not found")
		return nil, errors.New("Pull Request not found")
	}
	story, err := s.storyRepo.GetStoryById(int(pullRequest.StoryID))
	if err != nil {
		fmt.Println("Error fetching Story by ID")
		return nil, err
	}
	project, err := s.projectRepo.GetProjectById(int(story.ProjectID))
	if err != nil {
		fmt.Println("Error fetching Project by ID")
		return nil, err
	}
	spaceOrProjectName := s.gitService.GetSpaceOrProjectName(organisation)
	prResponse, err := s.gitService.FetchPullRequest(spaceOrProjectName, project.Name, pullRequest.PullRequestNumber)
	if err != nil {
		fmt.Println("Error fetching Pull Request by ID")
		return nil, err
	}
	mergeSHA, err := s.gitService.MergePullRequest(spaceOrProjectName, project.Name, pullRequest.PullRequestNumber, prResponse.SourceSHA)
	if err != nil {
		fmt.Println("Error merging pull request")
		return nil, err
	}
	fmt.Println("PR Merged Successfully")
	err = s.pullRequestRepo.UpdatePullRequestStatus(pullRequest, constants.Merged)
	return mergeSHA, nil
}

func (s *PullRequestService) GetPullRequestsCommits(pullRequestID int, organisationID int) ([]*response.GetAllCommitsResponse, error) {
	organisation, err := s.organisationRepo.GetOrganisationByID(uint(organisationID))
	pullRequest, err := s.pullRequestRepo.GetPullRequestByID(uint(pullRequestID))
	if err != nil {
		fmt.Println("Error fetching Pull Request by ID")
		return nil, err
	}
	if pullRequest == nil {
		fmt.Println("Pull Request not found")
		return nil, errors.New("Pull Request not found")
	}
	story, err := s.storyRepo.GetStoryById(int(pullRequest.StoryID))
	if err != nil {
		fmt.Println("Error fetching Story by ID")
		return nil, err
	}
	project, err := s.projectRepo.GetProjectById(int(story.ProjectID))
	if err != nil {
		fmt.Println("Error fetching Project by ID")
		return nil, err
	}
	spaceOrProjectName := s.gitService.GetSpaceOrProjectName(organisation)
	commitsResponse, err := s.gitService.FetchPullRequestCommits(spaceOrProjectName, project.Name, pullRequest.PullRequestNumber)
	commits, err := s.FetchCommitsResponse(commitsResponse)
	if err != nil {
		return nil, err
	}
	return commits, err
}

func (s *PullRequestService) GetPullRequestDiffByPullRequestID(pullRequestID uint) (string, error) {
	executionOutput, err := s.executionOutputRepo.GetExecutionOutputByID(pullRequestID)
	if err != nil {
		return "", err
	}

	if executionOutput == nil {
		return "", errors.New("execution output not found")
	}

	pullRequest, err := s.pullRequestRepo.GetPullRequestByID(pullRequestID)

	if err != nil {
		return "", err
	}

	if pullRequest == nil {
		return "", errors.New("execution output pull request not found")
	}

	execution, err := s.executionRepo.GetExecutionByID(executionOutput.ExecutionID)
	if err != nil {
		fmt.Println("Error getting execution by ID: ", err)
		return "", err
	}
	fmt.Printf("Execution: %v\n", execution)
	fmt.Println("Execution Story ID: ", execution.StoryID)
	story, err := s.storyRepo.GetStoryById(int(execution.StoryID))
	if err != nil {
		fmt.Println("Error getting story by ID: ", err)
		return "", err
	}
	fmt.Printf("Story: %v\n", story)
	project, err := s.projectRepo.GetProjectById(int(story.ProjectID))
	if err != nil {
		fmt.Println("Error getting project by ID: ", err)
		return "", err
	}

	//userName := "sample_user"
	organisation, err := s.organisationRepo.GetOrganisationByID(uint(int(project.OrganisationID)))
	spaceOrProjectName := s.gitService.GetSpaceOrProjectName(organisation)
	fmt.Printf("Project: %v\n", project)
	diff, err := s.gitService.GetPullRequestDiff(
		spaceOrProjectName,
		project.Name,
		pullRequest.MergeBaseSHA,
		pullRequest.SourceSHA,
	)
	if err != nil {
		return "", err
	}

	return diff, nil
}

func (s *PullRequestService) FetchCommitsResponse(commitResponse string) ([]*response.GetAllCommitsResponse, error) {
	var commits []map[string]interface{}
	err := json.Unmarshal([]byte(commitResponse), &commits)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return nil, err
	}
	var allCommitsResponse []*response.GetAllCommitsResponse

	// Parsing the commit data
	for _, commit := range commits {
		committer := commit["committer"].(map[string]interface{})
		identity := committer["identity"].(map[string]interface{})
		SHA := commit["sha"]
		when := committer["when"].(string)

		commitTime, err := time.Parse(time.RFC3339, when)
		if err != nil {
			fmt.Println("Error parsing commit time:", err)
			continue
		}
		currentTime := time.Now().UTC()
		allCommitsResponse = append(
			allCommitsResponse,
			&response.GetAllCommitsResponse{
				Title:    commit["title"].(string),
				Commiter: identity["name"].(string),
				SHA:      SHA.(string),
				Time:     utils.TimeAgo(commitTime, currentTime),
				Date:     commitTime.Format("02 January, 2006"),
			})

	}
	return allCommitsResponse, nil
}

func (s *PullRequestService) GetMergeDate(mergeDate time.Time, status string) string {
	if status == constants.Merged {
		return mergeDate.Format("Jan 2")
	}
	return ""
}

func (s *PullRequestService) GetClosedDate(mergeDate time.Time, status string) string {
	if status == constants.Close {
		return mergeDate.Format("Jan 2")
	}
	return ""
}

func (s *PullRequestService) GetTotalComments(pullRequestID uint) (int64, error) {
	commentsCount, err := s.pullRequestCommentsRepo.CountByPullRequestID(pullRequestID)
	if err != nil {
		fmt.Println(err.Error())
		return 0, err
	}
	return commentsCount, nil
}

func (s *PullRequestService) CreatePullRequest(prTitle, prDescription, prID, remoteType string, sourceSHA, mergeTargetSHA, mergeBaseSHA string, prNumber int, storyID uint, executionOutputId uint) (*models.PullRequest, error) {
	return s.pullRequestRepo.CreatePullRequest(prTitle, prDescription, prID, remoteType, sourceSHA, mergeTargetSHA, mergeBaseSHA, prNumber, storyID, executionOutputId)
}

func (s *PullRequestService) GetPullRequestByID(pullRequestId uint) (*models.PullRequest, error) {
	return s.pullRequestRepo.GetPullRequestByID(pullRequestId)
}

func (s *PullRequestService) UpdatePullRequestSourceSHA(pullRequest *models.PullRequest, sourceSHA string) error {
	return s.pullRequestRepo.UpdatePullRequestSourceSHA(pullRequest, sourceSHA)
}
