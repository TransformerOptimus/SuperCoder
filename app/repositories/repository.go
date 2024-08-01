package repositories

import "gorm.io/gorm"

type Repository struct {
	db *gorm.DB
}

func (repo *Repository) getRepositoryOptions(options ...RepositoryOption) *RepositoryOptions {
	repositoryOptions := &RepositoryOptions{
		db:    *repo.db,
		page:  1,
		limit: 10,
	}
	for _, option := range options {
		option.applyRepositoryOption(repositoryOptions)
	}
	return repositoryOptions
}
