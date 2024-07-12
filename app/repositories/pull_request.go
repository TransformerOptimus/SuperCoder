package repositories

import (
	"ai-developer/app/constants"
	"ai-developer/app/models"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type PullRequestRepository struct {
	db *gorm.DB
}

func NewPullRequestRepository(db *gorm.DB) *PullRequestRepository {
	return &PullRequestRepository{db: db}
}

func (r *PullRequestRepository) CreatePullRequest(prTitle, prDescription, prID, remoteType string,
	sourceSHA, mergeTargetSHA, mergeBaseSHA string, prNumber int, storyID uint, executionOutputId uint, prType string) (*models.PullRequest, error) {
	pullRequest := &models.PullRequest{
		StoryID:                storyID,
		PullRequestTitle:       prTitle,
		PullRequestNumber:      prNumber,
		Status:                 constants.Open,
		PullRequestDescription: prDescription,
		PullRequestID:          prID,
		RemoteType:             remoteType,
		SourceSHA:              sourceSHA,
		MergeTargetSHA:         mergeTargetSHA,
		MergeBaseSHA:           mergeBaseSHA,
		ExecutionOutputID:      executionOutputId,
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		PRType: 				prType,
	}
	if err := r.db.Create(pullRequest).Error; err != nil {
		return nil, err
	}
	return pullRequest, nil
}

func (r *PullRequestRepository) GetPullRequestByID(id uint) (*models.PullRequest, error) {
	var pullRequest *models.PullRequest
	fmt.Println("Pull Request ID : ", id)
	fmt.Println("db : ", r.db)
	err := r.db.First(&pullRequest, id).Error
	if err != nil {
		fmt.Println("Error : ", err)
		return nil, err
	}
	return pullRequest, nil
}

func (r *PullRequestRepository) UpdatePullRequestStatus(pullRequest *models.PullRequest, status string) error {
	pullRequest.Status = status
	if err := r.db.Save(pullRequest).Error; err != nil {
		return err
	}
	return nil
}

func (r* PullRequestCommentsRepository) UpdatePullRequestSourceSHA(pullRequest *models.PullRequest, source string) error{
	pullRequest.SourceSHA = source
    if err := r.db.Save(pullRequest).Error; err!= nil {
        return err
    }
    return nil
}

func (r *PullRequestRepository) GetAllPullRequestsByStoryIDs(storyIDs []uint, status string) ([]*models.PullRequest, error) {
	var pullRequests []*models.PullRequest
	query := r.db.Where("story_id IN (?)", storyIDs).Find(&pullRequests)

	if status != "ALL" {
		query = query.Where("status = ?", status)
	}

	query = query.Order("created_at DESC")

	err := query.Find(&pullRequests).Error
	if err != nil {
		return nil, err
	}
	return pullRequests, nil
}

func (r *PullRequestRepository) GetPullRequestByExecutionOutputId(executionOutputId uint) (*models.PullRequest, error) {
	var pullRequest *models.PullRequest
	err := r.db.First(&pullRequest, "execution_output_id = ?", executionOutputId).Error
	if err != nil {
		return nil, err
	}
	return pullRequest, nil
}

func (r *PullRequestRepository) UpdatePullRequestSourceSHA(pullRequest *models.PullRequest, sourceSHA string) error {
	pullRequest.SourceSHA = sourceSHA
	if err := r.db.Save(pullRequest).Error; err != nil {
		return err
	}
	return nil
}

func (receiver *PullRequestRepository) GetPullRequestsIdsByProjectAndStatus(projectStoryMap map[uint][]uint, status string) (map[int][]int, error) {
	var pullRequests []models.PullRequest
	storyIds := []uint{}
	for _, ids := range projectStoryMap {
		storyIds = append(storyIds, ids...)
	}
	err := receiver.db.Where("story_id IN (?) AND status = ?", storyIds, status).Find(&pullRequests).Error
	if err != nil {
		return nil, err
	}
	storyPullRequestMap := make(map[uint][]int)
	for _, pullRequest := range pullRequests {
		storyPullRequestMap[pullRequest.StoryID] = append(storyPullRequestMap[pullRequest.StoryID], int(pullRequest.ID))
	}
	projectPullRequestMap := make(map[int][]int)
	for projectId, storyIDs := range projectStoryMap {
		for _, storyID := range storyIDs {
			projectPullRequestMap[int(projectId)] = append(projectPullRequestMap[int(projectId)], storyPullRequestMap[storyID]...)
		}
	}
	return projectPullRequestMap, nil
}

func (r *PullRequestRepository) GetPullRequestWithDetails(pullRequestID uint) (*models.Project, error) {
	var project models.Project
	err := r.db.Raw(
		"SELECT projects.* FROM pull_requests "+
			"JOIN stories ON stories.id = pull_requests.story_id "+
			"JOIN projects ON projects.id = stories.project_id "+
			"WHERE pull_requests.id = ?", pullRequestID,
	).Scan(&project).Error

	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *PullRequestRepository) GetOpenPullRequestsByStoryID(storyID int) (*models.PullRequest, error) {
	var pullRequest *models.PullRequest
	err := r.db.Where("story_id =? AND status =?", storyID, constants.Open).First(&pullRequest).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return pullRequest, nil
}
