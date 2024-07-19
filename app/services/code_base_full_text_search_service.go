package services

import (
	"ai-developer/app/repositories/search"
	"context"
)

type CodeBaseSearchService struct {
	codeFullTextSearchRepository search.CodeBaseSearchRepository
}

func NewCodeBaseSearchService(codeFullTextSearchRepository search.CodeBaseSearchRepository) *CodeBaseSearchService {
	return &CodeBaseSearchService{codeFullTextSearchRepository: codeFullTextSearchRepository}
}

func (s *CodeBaseSearchService) IndexDocument(ctx context.Context, document interface{}) error {
	return s.codeFullTextSearchRepository.IndexDocument(ctx, document)
}

func (s *CodeBaseSearchService) SearchDocument(ctx context.Context, query interface{}) ([]interface{}, error) {
	return s.codeFullTextSearchRepository.Search(ctx, query)
}
