package repositories

import (
	"ai-developer/app/constants"
	"ai-developer/app/models"
	"fmt"
	"gorm.io/gorm"
)

type StoryRepository struct {
	db *gorm.DB
}

func (receiver *StoryRepository) CreateStory(story *models.Story) (*models.Story, error) {
	err := receiver.db.Create(story).Error
	if err != nil {
		return nil, err
	}
	return story, nil
}

func (receiver *StoryRepository) GetStoriesByProjectId(projectId int, storyType string) ([]models.Story, error) {
	var stories []models.Story
	err := receiver.db.Where("project_id = ? AND is_deleted = ? AND type = ?", projectId, false, storyType).Find(&stories).Error
	if err != nil {
		return nil, err
	}
	return stories, nil
}

func (receiver *StoryRepository) GetStoriesByProjectIdAndSearch(projectId int, searchValue string, storyType string) ([]models.Story, error) {
    var stories []models.Story
    searchPattern := searchValue + "%"
    err := receiver.db.Where("title ILIKE ? AND project_id = ? AND is_deleted = ? AND type = ?", 
        searchPattern, projectId, false, storyType).Find(&stories).Error
    if err != nil {
        return nil, err
    }
    return stories, nil
}

func (receiver *StoryRepository) GetStoryIdsMapByProjectIds(projectIds []int) (map[uint][]uint, error) {
	var stories []models.Story
	err := receiver.db.Where("project_id IN (?) AND is_deleted = ?", projectIds, false).Find(&stories).Error
	if err != nil {
		return nil, err
	}
	projectStoryMap := make(map[uint][]uint)
	for _, story := range stories {
		projectStoryMap[story.ProjectID] = append(projectStoryMap[story.ProjectID], story.ID)
	}
	return projectStoryMap, nil
}

func (r *StoryRepository) GetStoryByExecutionID(executionID uint) (*models.Story, error) {
	var execution models.Execution
	if err := r.db.First(&execution, executionID).Error; err != nil {
		return nil, err
	}
	return r.GetStoryById(int(execution.StoryID))
}

func (receiver *StoryRepository) GetStoryByProjectIdAndStatus(projectId int, status string) (*models.Story, error) {
	var story models.Story
	err := receiver.db.Where("project_id = ? AND status = ? AND is_deleted = ?", projectId, status, false).First(&story).Error
	if err != nil {
		return nil, err
	}
	return &story, nil
}

func (receiver *StoryRepository) GetStoryById(id int) (*models.Story, error) {
	var story models.Story
	err := receiver.db.First(&story, id).Error
	if err != nil {
		return nil, err
	}
	return &story, nil
}

func (receiver *StoryRepository) UpdateStory(story *models.Story, summary, description string) error {
	story.Title = summary
	story.Description = description
	err := receiver.db.Save(story).Error
	if err != nil {
		return err
	}
	return nil
}

func (receiver *StoryRepository) UpdateStoryStatus(story *models.Story, status string) error {
	story.Status = status
	err := receiver.db.Save(story).Error
	if err != nil {
		return err
	}
	return nil
}

func (receiver *StoryRepository) UpdateReviewViewedStatus(story *models.Story, viewedStatus bool) error {
	story.ReviewViewed = viewedStatus
	err := receiver.db.Save(story).Error
	if err != nil {
		return err
	}
	return nil
}

func (receiver *StoryRepository) GetInProgressStoriesByProjectId(projectId int) ([]*models.Story, error) {
	var stories []*models.Story
	err := receiver.db.Where("project_id = ? AND is_deleted = ? AND status = ?", projectId, false, constants.InProgress).Find(&stories).Error
	if err != nil {
		return nil, err
	}
	return stories, nil
}

func (receiver *StoryRepository) UpdateStatus(storyId int, status string) (*models.Story, error) {

	fmt.Println("Updating Status : ", status)

	var story models.Story
	if err := receiver.db.First(&story, storyId).Error; err != nil {
		fmt.Printf("Error fetching story ", err.Error())
		return nil, err
	}
	fmt.Printf("Updating Story : ", story)
	story.Status = status
	if err := receiver.db.Save(&story).Error; err != nil {
		fmt.Printf("Error saving story : ", err.Error())
		return nil, err
	}
	return &story, nil
}

func (receiver *StoryRepository) DeleteStoryById(story *models.Story) error {
	fmt.Println("Deleting Story")
	story.IsDeleted = true
	err := receiver.db.Save(story).Error
	if err != nil {
		return err
	}
	return nil
}

// UpdateStoryStatusWithTx updates the status of a story with a transaction by its ID.
func (receiver *StoryRepository) UpdateStoryStatusWithTx(tx *gorm.DB, storyID int, status string) error {
	var story models.Story
	if err := tx.First(&story, storyID).Error; err != nil {
		return err
	}
	story.Status = status
	if err := tx.Save(&story).Error; err != nil {
		return err
	}
	return nil
}

func NewStoryRepository(db *gorm.DB) *StoryRepository {
	return &StoryRepository{
		db: db,
	}
}
