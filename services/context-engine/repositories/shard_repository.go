package repositories

import (
	"context"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
)

type ShardRepository interface {
	AssignShard(ctx context.Context, repoID uint) (*postgres.ShardAssignment, error)
	GetAssignment(ctx context.Context, repoID uint) (*postgres.ShardAssignment, error)
	GetCollectionUsageCounts(ctx context.Context) (map[string]int64, error)
}
