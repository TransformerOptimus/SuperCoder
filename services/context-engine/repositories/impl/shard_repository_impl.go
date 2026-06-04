package impl

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/db/postgres"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
)

type shardRepositoryImpl struct {
	db     *gorm.DB
	cfg    config.ShardConfig
	logger *zap.Logger
}

func NewShardRepository(db *gorm.DB, cfg config.ShardConfig, logger *zap.Logger) repositories.ShardRepository {
	return &shardRepositoryImpl{
		db:     db,
		cfg:    cfg,
		logger: logger.Named("shard-repo"),
	}
}

func (s *shardRepositoryImpl) AssignShard(ctx context.Context, repoID uint) (*postgres.ShardAssignment, error) {
	var existing postgres.ShardAssignment
	if err := s.db.WithContext(ctx).Where("repo_id = ?", repoID).First(&existing).Error; err == nil {
		return &existing, nil
	}

	counts, err := s.GetCollectionUsageCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get collection usage counts: %w", err)
	}

	leastLoaded, err := s.findLeastLoadedShard(counts)
	if err != nil {
		return nil, fmt.Errorf("find least loaded shard: %w", err)
	}

	assignment := &postgres.ShardAssignment{
		RepoID:         repoID,
		CollectionName: leastLoaded,
	}

	if err := s.db.WithContext(ctx).Create(assignment).Error; err != nil {
		var existing postgres.ShardAssignment
		if err2 := s.db.WithContext(ctx).Where("repo_id = ?", repoID).First(&existing).Error; err2 == nil {
			return &existing, nil
		}
		return nil, fmt.Errorf("failed to assign shard: %w", err)
	}

	s.logger.Info("Assigned shard",
		zap.Uint("repo_id", repoID),
		zap.String("collection", leastLoaded))

	return assignment, nil
}

func (s *shardRepositoryImpl) GetAssignment(ctx context.Context, repoID uint) (*postgres.ShardAssignment, error) {
	var assignment postgres.ShardAssignment
	if err := s.db.WithContext(ctx).Where("repo_id = ?", repoID).First(&assignment).Error; err != nil {
		return nil, fmt.Errorf("shard assignment not found for repo %d: %w", repoID, err)
	}
	return &assignment, nil
}

func (s *shardRepositoryImpl) GetCollectionUsageCounts(ctx context.Context) (map[string]int64, error) {
	type countResult struct {
		CollectionName string
		Count          int64
	}

	var results []countResult
	if err := s.db.WithContext(ctx).
		Model(&postgres.ShardAssignment{}).
		Select("collection_name, count(*) as count").
		Group("collection_name").
		Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get usage counts: %w", err)
	}

	counts := make(map[string]int64)
	for _, r := range results {
		counts[r.CollectionName] = r.Count
	}
	return counts, nil
}

func (s *shardRepositoryImpl) findLeastLoadedShard(counts map[string]int64) (string, error) {
	prefix := s.cfg.ShardPrefix()
	count := s.cfg.ShardCount()

	if count <= 0 {
		return "", fmt.Errorf("shard count must be positive, got %d", count)
	}

	var bestName string
	var bestCount int64 = -1

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("%s_%04d", prefix, i)
		c := counts[name]
		if bestCount < 0 || c < bestCount {
			bestName = name
			bestCount = c
		}
	}

	return bestName, nil
}
