package repository

import "context"

type SearchRepository interface {
	IndexDocument(ctx context.Context, index string, document interface{}) error
	Search(ctx context.Context, index string, query interface{}) ([]interface{}, error)
}
