package producers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/consumers"
)

// IndexForReviewRequest carries the identity needed to enqueue an index task
// kicked off from a review webhook. The repo+commit pair is used as the
// deterministic TaskID so multiple concurrent review events for the same
// commit coalesce to one index run.
type IndexForReviewRequest struct {
	Repository   string
	HeadSHA      string
	WorkspaceID  uint64
	UserID       string
	SourceType   string
	S3Bucket     string
	S3Prefix     string
}

type IndexProducer struct {
	logger *zap.Logger
	client *asynq.Client
}

func NewIndexProducer(logger *zap.Logger, client *asynq.Client) *IndexProducer {
	return &IndexProducer{
		logger: logger.Named("producers.index"),
		client: client,
	}
}

func (p *IndexProducer) EnqueueIndex(ctx context.Context, repoPath, repoURL string, reindex bool, userID string, workspaceID uint64, machineID, sourceType, s3Bucket, s3Prefix string) (string, error) {
	payload := consumers.IndexRepoPayload{
		RepoPath:    repoPath,
		RepoURL:     repoURL,
		Reindex:     reindex,
		UserID:      userID,
		WorkspaceID: workspaceID,
		MachineID:   machineID,
		SourceType:  sourceType,
		S3Bucket:    s3Bucket,
		S3Prefix:    s3Prefix,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := asynq.NewTask(consumers.TypeIndexRepo, payloadBytes,
		asynq.MaxRetry(0),
	)
	info, err := p.client.EnqueueContext(ctx, task)
	if err != nil {
		p.logger.Error("Failed to enqueue index task",
			zap.String("repo_path", repoPath),
			zap.Error(err))
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	p.logger.Info("Enqueued index task",
		zap.String("task_id", info.ID),
		zap.String("repo_path", repoPath))

	return info.ID, nil
}

// EnqueueIndexForReview schedules an index run kicked off by a review webhook.
// The TaskID is deterministic — `index:review:<repo>:<head-sha>` — so
// duplicate review events for the same commit are dropped at the queue level
// rather than triggering redundant embedding work. Returns the task ID; on
// TaskIDConflict the already-queued/running task ID is returned with no error.
func (p *IndexProducer) EnqueueIndexForReview(ctx context.Context, req IndexForReviewRequest) (string, error) {
	payload := consumers.IndexRepoPayload{
		RepoPath:    req.Repository,
		UserID:      req.UserID,
		WorkspaceID: req.WorkspaceID,
		SourceType:  req.SourceType,
		S3Bucket:    req.S3Bucket,
		S3Prefix:    req.S3Prefix,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	taskID := fmt.Sprintf("index:review:%s:%s", req.Repository, req.HeadSHA)
	task := asynq.NewTask(consumers.TypeIndexRepo, payloadBytes,
		asynq.MaxRetry(1),
		asynq.TaskID(taskID),
	)
	info, err := p.client.EnqueueContext(ctx, task)
	if err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) {
			p.logger.Info("duplicate index task — already queued or running",
				zap.String("task_id", taskID),
				zap.String("repo", req.Repository),
				zap.String("head_sha", req.HeadSHA))
			return taskID, nil
		}
		p.logger.Error("failed to enqueue review-triggered index task",
			zap.String("task_id", taskID),
			zap.String("repo", req.Repository),
			zap.String("head_sha", req.HeadSHA),
			zap.Error(err))
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	p.logger.Info("enqueued review-triggered index task",
		zap.String("task_id", info.ID),
		zap.String("repo", req.Repository),
		zap.String("head_sha", req.HeadSHA))

	return info.ID, nil
}

func (p *IndexProducer) EnqueueAudit(ctx context.Context, repoPath string) (string, error) {
	payload := consumers.AuditRepoPayload{
		RepoPath: repoPath,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := asynq.NewTask(consumers.TypeAuditRepo, payloadBytes,
		asynq.MaxRetry(0),
		asynq.Unique(10*time.Minute),
	)
	info, err := p.client.EnqueueContext(ctx, task)
	if err != nil {
		p.logger.Error("Failed to enqueue audit task",
			zap.String("repo_path", repoPath),
			zap.Error(err))
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	p.logger.Info("Enqueued audit task",
		zap.String("task_id", info.ID),
		zap.String("repo_path", repoPath))

	return info.ID, nil
}
