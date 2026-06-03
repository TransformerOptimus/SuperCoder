package impl

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
)

type repoRepositoryImpl struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewRepoRepository(db *gorm.DB, logger *zap.Logger) repositories.RepoRepository {
	return &repoRepositoryImpl{
		db:     db,
		logger: logger.Named("repo-repo"),
	}
}

func (r *repoRepositoryImpl) FindOrCreate(ctx context.Context, userID string, workspaceID uint64, machineID, repoPath, repoURL string) (*postgres.Repo, error) {
	repo := &postgres.Repo{
		UserID:      userID,
		WorkspaceID: workspaceID,
		MachineID:   machineID,
		RepoPath:    repoPath,
		RepoURL:     repoURL,
	}

	result := r.db.WithContext(ctx).
		Where("user_id = ? AND workspace_id = ? AND machine_id = ? AND repo_path = ?",
			userID, workspaceID, machineID, repoPath).
		FirstOrCreate(repo)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to find or create repo: %w", result.Error)
	}

	if repoURL != "" && repo.RepoURL != repoURL {
		if err := r.db.WithContext(ctx).Model(repo).Update("repo_url", repoURL).Error; err != nil {
			return nil, fmt.Errorf("failed to update repo url: %w", err)
		}
		repo.RepoURL = repoURL
	}

	return repo, nil
}
