package repositories

import (
	"ai-developer/app/models"
	"gorm.io/gorm"
	"log"
	"time"
)

type PullRequestCommentsRepository struct {
	db *gorm.DB
}

func NewPullRequestCommentsRepository(db *gorm.DB) *PullRequestCommentsRepository {
	return &PullRequestCommentsRepository{db: db}
}

func (r *PullRequestCommentsRepository) CountByPullRequestID(pullRequestID uint) (int64, error) {
	var count int64
	result := r.db.Model(&models.PullRequestComments{}).Where("pull_request_id = ?", pullRequestID).Count(&count)
	if result.Error != nil {
		log.Fatal("failed to count stories:", result.Error)
	}
	return count, nil
}

func (r *PullRequestCommentsRepository) CreateComment(pullRequestID uint, comment string) error {
	pullRequestComment := &models.PullRequestComments{
		PullRequestID: pullRequestID,
		Comment:       comment,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := r.db.Create(pullRequestComment).Error; err != nil {
		return err
	}
	return nil
}

func (r *PullRequestCommentsRepository) GetAllCommentsByPullRequestID(pullRequestID uint) ([]models.PullRequestComments, error) {
	var comments []models.PullRequestComments
	result := r.db.Where("pull_request_id = ?", pullRequestID).Find(&comments)
	if result.Error != nil {
		return nil, result.Error
	}
	return comments, nil
}
