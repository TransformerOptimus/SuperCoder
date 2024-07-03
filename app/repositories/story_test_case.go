package repositories

import (
	"ai-developer/app/models"
	"gorm.io/gorm"
	"log"
)

type StoryTestCaseRepository struct {
	db *gorm.DB
}

func (receiver *StoryTestCaseRepository) CreateStoryTestCase(s *models.StoryTestCase) error {
	err := receiver.db.Create(s).Error
	if err != nil {
		return err
	}
	return nil
}

func (receiver StoryTestCaseRepository) GetStoryTestCaseByStoryId(storyId int) ([]models.StoryTestCase, error) {
	var storyTestCase []models.StoryTestCase
	err := receiver.db.Where("story_id = ?", storyId).Find(&storyTestCase).Error
	if err != nil {
		return nil, err
	}
	return storyTestCase, nil
}

func (receiver *StoryTestCaseRepository) DeleteStoryTestCaseByID(storyTestCase *models.StoryTestCase) error {
	if err := receiver.db.Delete(&storyTestCase).Error; err != nil {
		log.Fatal(err)
		return err
	}
	return nil
}

func (receiver *StoryTestCaseRepository) GetStoryTestCaseById(id int) (*models.StoryTestCase, error) {
	var storyTestCase models.StoryTestCase
	err := receiver.db.Where("id = ?", id).First(&storyTestCase).Error
	if err != nil {
		return nil, err
	}
	return &storyTestCase, nil
}

func NewStoryTestCaseRepository(db *gorm.DB) *StoryTestCaseRepository {
	return &StoryTestCaseRepository{
		db: db,
	}
}
