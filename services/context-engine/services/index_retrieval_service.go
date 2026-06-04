package services

import (
	"context"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/models/dto"
)

type IndexRetrievalService interface {
	Search(ctx context.Context, req *dto.SearchRequest) (*dto.SearchResponse, error)
	GraphQuery(ctx context.Context, req *dto.GraphQueryRequest) (*dto.GraphQueryResponse, error)
	GetContext(ctx context.Context, req *dto.ContextRequest) (*dto.ContextResponse, error)
	GetIndexStatus(ctx context.Context, repoPath, userID string, workspaceID uint64, machineID string) (*dto.IndexStatusResponse, error)
	DeleteIndex(ctx context.Context, req *dto.IndexDeleteRequest) (*dto.IndexDeleteResponse, error)
	TriggerIndex(ctx context.Context, req *dto.IndexRequest) (string, error)
}
