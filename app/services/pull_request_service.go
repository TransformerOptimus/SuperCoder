package services

import (
	"ai-developer/app/config"
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
	"strconv"
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
	storyType := "backend"
	stories, err := s.storyRepo.GetStoriesByProjectId(projectID, storyType)
	// fmt.Println("____stories_____", stories)
	if err != nil {
		return nil, err
	}
	storyIDs := make([]uint, len(stories))
	for i, story := range stories {
		storyIDs[i] = story.ID
	}
	pullRequests, err := s.pullRequestRepo.GetAllPullRequestsByStoryIDs(storyIDs, status)
	// fmt.Println("____pullRequests_____", pullRequests)
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
	if err!=nil{
		fmt.Println("Error fetching Organisation by ID")
        return nil, err
	}
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
	if err!=nil{
		return nil, err
	}
	commits, err := s.FetchCommitsResponse(commitsResponse)
	if err != nil {
		return nil, err
	}
	return commits, err
}

func (s *PullRequestService) GetPullRequestDiffByPullRequestID(pullRequestID uint) (string, error) {
	pullRequest, err := s.pullRequestRepo.GetPullRequestByID(pullRequestID)
	if err!=nil{
		return "", err
	}
	story, err := s.storyRepo.GetStoryById(int(pullRequest.StoryID))
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

	organisation, err := s.organisationRepo.GetOrganisationByID(uint(int(project.OrganisationID)))
	if err!= nil {
        fmt.Println("Error getting organisation by ID: ", err)
        return "", err
    }
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

func (s *PullRequestService) CreatePullRequest(prTitle, prDescription, prID, remoteType string, sourceSHA, mergeTargetSHA, mergeBaseSHA string, prNumber int, storyID uint, executionOutputId uint, prType string) (*models.PullRequest, error) {
	return s.pullRequestRepo.CreatePullRequest(prTitle, prDescription, prID, remoteType, sourceSHA, mergeTargetSHA, mergeBaseSHA, prNumber, storyID, executionOutputId, prType)
}

func (s *PullRequestService) GetPullRequestByID(pullRequestId uint) (*models.PullRequest, error) {
	return s.pullRequestRepo.GetPullRequestByID(pullRequestId)
}

func (s *PullRequestService) UpdatePullRequestSourceSHA(pullRequest *models.PullRequest, sourceSHA string) error {
	return s.pullRequestRepo.UpdatePullRequestSourceSHA(pullRequest, sourceSHA)
}
func (s *PullRequestService) CreatePullRequestFromCodeEditor(projectID int, title string, description string) (int, error){
	project, err := s.projectRepo.GetProjectById(projectID)
	if err!= nil {
		fmt.Println("failed to fetch project", err)
        return -1, err
    }

	workingDir := "/workspaces/"+project.HashID
	err = utils.ConfigureGitUserName(workingDir)
	if err!= nil {
        fmt.Println("failed to configure git user name", err)
        return -1, err
    }
	err = utils.ConfigGitUserEmail(workingDir)
	if err!= nil {
        fmt.Println("failed to configure git user email", err)
        return -1, err
    }
	err = utils.ConfigGitSafeDir(workingDir)
	if err!= nil {
        fmt.Println("failed to configure git safe dir", err)
        return -1, err
    }

	currentBranch, err := utils.GetCurrentBranch(workingDir)
	if err!=nil{
		fmt.Println("failed to get current branch", err)
        return -1, err
	}
	fmt.Printf("-------Current branch: %s----- ", currentBranch)
	if currentBranch=="main"{
		return -1, errors.New("current branch is main can not raise a pr")
	}
	execution, err := s.executionRepo.GetExecutionsByBranchName(currentBranch)
	if err!= nil {
        fmt.Println("failed to fetch executions by branch name", err)
        return -1, err
    }
	storyID := execution.StoryID

	output, err := utils.GitAddToTrackFiles(workingDir, nil)
	if err != nil {
		fmt.Printf("Error adding files to track: %s\n", err.Error())
		return -1, err
	}
	fmt.Printf("Git add output: %s\n", output)

	commitMsg := fmt.Sprintf("commiting for project id: %s\n", strconv.Itoa(projectID))
	output, err =utils.GitCommitWithMessage(
		workingDir,
		commitMsg,
		nil,
	)
	fmt.Printf("Git commit output: %s\n", output)
	if err != nil {
		fmt.Printf("Error commiting code: %s\n", err.Error())
		return -1, err
	}

	organisationID := project.OrganisationID
	organisation, err := s.organisationRepo.GetOrganisationByID(uint(organisationID))
	if err!= nil {
		fmt.Println("failed to fetch organisation", err)
		return -1, err
	}
	spaceOrProjectName := s.gitService.GetSpaceOrProjectName(organisation)
	openPullRequest, err := s.pullRequestRepo.GetOpenPullRequestsByStoryID(int(storyID))
	if err!= nil {
        fmt.Println("failed to fetch open pull requests by story id", err)
        return -1, err
    }

	httpPrefix := "https"
	if config.AppEnv() == constants.Development {
		httpPrefix = "http"
	}
	origin := fmt.Sprintf("%s://%s:%s@%s/git/%s/%s.git", httpPrefix, config.GitnessUser(), config.GitnessToken(), config.GitnessHost(), spaceOrProjectName, project.Name)
	err = utils.GitPush(workingDir, origin, currentBranch)
	if err!=nil{
		fmt.Printf("Error pushing changes: %s\n", err.Error())
		return -1, err
	}

	if openPullRequest == nil {
		err := utils.PullOriginMain(workingDir, origin)
		if err!= nil {
            fmt.Printf("Error pulling origin main: %s\n", err.Error())
            return -1, err
        }
		fmt.Println("____no open pull requests, creating a new one____")
		pr, err := s.gitService.CreatePullRequest(spaceOrProjectName, project.Name, currentBranch, "main", "Pull Request: "+title, description)
		if err != nil {
			fmt.Printf("Error creating pull request: %s\n", err.Error())
			return -1, err
		}
		prType := "manual"
		pullRequest, err := s.CreatePullRequest(pr.Title, pr.Description, strconv.Itoa(pr.Number), "GITNESS", pr.SourceSHA, "sample", pr.MergeBaseSHA, pr.Number, storyID, 0, prType)
		if err!= nil {
			fmt.Printf("Error creating pull request in database: %s\n", err.Error())
			return -1, err
		}
		fmt.Println("Pull Request created successfully", pullRequest)

		err = utils.ConfigGitSafeDir("/workspaces")
		if err!= nil {
			fmt.Println("failed to configure git safe dir", err)
			return -1, err
		}

		return int(pullRequest.ID), nil
	} else {
		fmt.Println("______found an open pull request pushing changes in it______")
		latestCommitID, err := utils.GetLatestCommitID(workingDir, err)
		if err!= nil{
            fmt.Printf("Error getting latest commit id: %s\n", err.Error())
            return -1, err
        }
		err = s.UpdatePullRequestSourceSHA(openPullRequest, latestCommitID)
		if err!= nil {
            fmt.Printf("Error updating pull request source sha: %s\n", err.Error())
            return -1, err
        }

		err = utils.ConfigGitSafeDir("/workspaces")
		if err!= nil {
			fmt.Println("failed to configure git safe dir", err)
			return -1, err
		}
		
		return int(openPullRequest.ID), nil
	}
}