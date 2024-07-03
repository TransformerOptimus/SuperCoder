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
