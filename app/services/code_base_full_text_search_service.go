package services

import (
	fullTextSearchRepository "ai-developer/app/repositories/interface"
	"context"
)

type CodeBaseSearchService struct {
	repo fullTextSearchRepository.CodeBaseSearchRepository
}

func NewCodeBaseSearchService(repo fullTextSearchRepository.CodeBaseSearchRepository) *CodeBaseSearchService {
	return &CodeBaseSearchService{repo: repo}
}

func (s *CodeBaseSearchService) IndexDocument(ctx context.Context, index string, document interface{}) error {
	return s.repo.IndexDocument(ctx, index, document)
}

func (s *CodeBaseSearchService) SearchDocument(ctx context.Context, index string, query interface{}) ([]interface{}, error) {
	return s.repo.Search(ctx, index, query)
}
