package repositories

import (
	"ai-developer/app/models"
	"gorm.io/gorm"
)

type StoryInstructionRepository struct {
	db *gorm.DB
}

func (receiver StoryInstructionRepository) CreateStoryInstructions(s *models.StoryInstruction) error {
	err := receiver.db.Create(s).Error
	if err != nil {
		return err
	}
	return nil
}

func (receiver StoryInstructionRepository) GetStoryInstructionByStoryId(storyId int) ([]models.StoryInstruction, error) {
	var storyInstruction []models.StoryInstruction
	err := receiver.db.Where("story_id = ?", storyId).Find(&storyInstruction).Error
	if err != nil {
		return nil, err
	}
	return storyInstruction, nil
}

func (receiver StoryInstructionRepository) GetOneStoryInstructionByStoryId(storyId int) (*models.StoryInstruction, error) {
	var storyInstruction *models.StoryInstruction
	err := receiver.db.Where("story_id = ?", storyId).First(&storyInstruction).Error
	if err != nil {
		return nil, err
	}
	return storyInstruction, nil
}

func (receiver StoryInstructionRepository) UpdateStoryInstructions(storyInstruction *models.StoryInstruction, newInstruction string) error {
	storyInstruction.Instruction = newInstruction
	err := receiver.db.Save(storyInstruction).Error
	if err != nil {
		return err
	}
	return nil
}

func NewStoryInstructionRepository(db *gorm.DB) *StoryInstructionRepository {
	return &StoryInstructionRepository{
		db: db,
	}
}
