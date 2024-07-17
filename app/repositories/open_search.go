package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/opensearch-project/opensearch-go"
	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"go.uber.org/zap"
	"io"
	"strings"
)

type OpenSearchRepository struct {
	client *opensearch.Client
	logger *zap.Logger
}

func NewOpenSearchRepository(client *opensearch.Client, logger *zap.Logger) *OpenSearchRepository {
	return &OpenSearchRepository{
		client: client,
		logger: logger.Named("OpenSearchRepository"),
	}
}

func (r *OpenSearchRepository) IndexDocument(ctx context.Context, index string, document interface{}) error {
	body, err := json.Marshal(document)
	if err != nil {
		return err
	}

	req := opensearchapi.IndexRequest{
		Index:   index,
		Body:    strings.NewReader(string(body)),
		Refresh: "true",
	}

	fmt.Println(r.client.UpdateByQueryRethrottle)

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println("Error closing response body")
		}
	}(res.Body)

	if res.IsError() {
		return fmt.Errorf("error indexing document: %s", res.String())
	}

	return nil
}

func (r *OpenSearchRepository) Search(ctx context.Context, index string, query interface{}) ([]interface{}, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}

	req := opensearchapi.SearchRequest{
		Index: []string{index},
		Body:  strings.NewReader(string(body)),
	}

	res, err := req.Do(ctx, r.client)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println("Error closing response body")
		}
	}(res.Body)

	if res.IsError() {
		return nil, fmt.Errorf("error searching documents: %s", res.String())
	}

	var rmap map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&rmap); err != nil {
		return nil, err
	}

	hits := rmap["hits"].(map[string]interface{})["hits"].([]interface{})
	documents := make([]interface{}, len(hits))
	for i, hit := range hits {
		documents[i] = hit.(map[string]interface{})["_source"]
	}

	return documents, nil
}
