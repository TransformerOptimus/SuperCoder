package search

import "context"

type CodeBaseSearchRepository interface {
	IndexDocument(ctx context.Context, document interface{}) error
	Search(ctx context.Context, query interface{}) ([]interface{}, error)
}
