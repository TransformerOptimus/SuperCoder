package repositories

import (
	"context"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
)

type RepoRepository interface {
	FindOrCreate(ctx context.Context, userID string, workspaceID uint64, machineID, repoPath, repoURL string) (*postgres.Repo, error)
}
