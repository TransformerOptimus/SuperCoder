package services

import (
	fullTextSearchRepository "ai-developer/app/repositories/interface"
	"context"
)

type SearchService struct {
	repo fullTextSearchRepository.SearchRepository
}

func NewSearchService(repo fullTextSearchRepository.SearchRepository) *SearchService {
	return &SearchService{repo: repo}
}

func (s *SearchService) IndexDocument(ctx context.Context, index string, document interface{}) error {
	return s.repo.IndexDocument(ctx, index, document)
}

func (s *SearchService) SearchDocument(ctx context.Context, index string, query interface{}) ([]interface{}, error) {
	return s.repo.Search(ctx, index, query)
}
