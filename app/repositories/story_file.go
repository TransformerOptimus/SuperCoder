package repositories

import (
	"ai-developer/app/models"
	"gorm.io/gorm"
)

type StoryFileRepository struct {
	db *gorm.DB
}

func NewStoryFileRepository(db *gorm.DB) *StoryFileRepository {
	return &StoryFileRepository{
		db: db,
	}
}

func (storyFile *StoryFileRepository) GetFilesByStoryID(storyID uint) ([]models.StoryFile, error) {
	var files []models.StoryFile
	err := storyFile.db.Where("story_id = ?", storyID).Find(&files).Error
	return files, err
}

func (receiver *StoryFileRepository) CreateStoryFile(storyFile *models.StoryFile) error {
	err := receiver.db.Create(storyFile).Error
	if err != nil {
		return err
	}
	return nil
}

func (storyFile *StoryFileRepository) GetFileByStoryID(storyID uint) (*models.StoryFile, error) {
	var file models.StoryFile
	err := storyFile.db.Where("story_id = ?", storyID).First(&file).Error
	return &file, err
}

func (repository *StoryFileRepository) UpdateStoryFileUrl(storyFile *models.StoryFile, s3Url string) error {
	storyFile.FilePath = s3Url
	err := repository.db.Save(storyFile).Error
	if err != nil {
		return err
	}
	return nil
}
