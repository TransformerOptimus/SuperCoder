package repositories

import (
	"ai-developer/app/models"
	"gorm.io/gorm"
)

type DesignStoryReviewRepository struct {
	db *gorm.DB
}

func NewDesignStoryReviewRepository(db *gorm.DB) *DesignStoryReviewRepository {
	return &DesignStoryReviewRepository{db: db}
}

func (r *DesignStoryReviewRepository) CreateDesignStoryReview(designStoryReview *models.DesignStoryReview) error {
	if err := r.db.Create(designStoryReview).Error; err != nil {
		return err
	}
	return nil
}

func (r *DesignStoryReviewRepository) GetAllDesignReviewsByStoryId(storyId uint) ([]*models.DesignStoryReview, error) {
	var designStoryReviews []*models.DesignStoryReview
	if err := r.db.Where("story_id = ?", storyId).Find(&designStoryReviews).Error; err != nil {
		return nil, err
	}
	return designStoryReviews, nil
}
