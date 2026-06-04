package impl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/repositories"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type vectorRepositoryImpl struct {
	client *qdrant.Client
	logger *zap.Logger
}

func NewVectorRepository(cfg config.QdrantConfig, logger *zap.Logger) (repositories.VectorRepository, error) {
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: cfg.Host(),
		Port: cfg.Port(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Qdrant: %w", err)
	}
	return &vectorRepositoryImpl{
		client: client,
		logger: logger.Named("vector-repo"),
	}, nil
}

func (v *vectorRepositoryImpl) EnsureCollection(ctx context.Context, collection string, dim uint64) error {
	exists, err := v.client.CollectionExists(ctx, collection)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	err = v.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: collection,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     dim,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}

	// Wait until collection is fully operational by probing with a real Scroll call.
	// CollectionExists returns true before shards are initialized, so we must verify
	// the collection can actually serve requests before returning.
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		_, scrollErr := v.client.Scroll(ctx, &qdrant.ScrollPoints{
			CollectionName: collection,
			Limit:          qdrant.PtrOf(uint32(1)),
		})
		if scrollErr == nil {
			v.logger.Debug("Collection ready", zap.String("collection", collection), zap.Int("probe_attempts", i+1))
			return nil
		}
	}
	return fmt.Errorf("collection %q created but not ready after 15s", collection)
}

func (v *vectorRepositoryImpl) EnsurePayloadIndexes(ctx context.Context, collection string) error {
	indexes := []struct {
		field     string
		fieldType qdrant.FieldType
	}{
		{"user_id", qdrant.FieldType_FieldTypeKeyword},
		{"workspace_id", qdrant.FieldType_FieldTypeKeyword},
		{"machine_id", qdrant.FieldType_FieldTypeKeyword},
		{"repo_id", qdrant.FieldType_FieldTypeInteger},
		{"github_org_id", qdrant.FieldType_FieldTypeKeyword},
		{"file_path", qdrant.FieldType_FieldTypeKeyword},
	}

	for _, idx := range indexes {
		if _, err := v.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
			CollectionName: collection,
			FieldName:      idx.field,
			FieldType:      &idx.fieldType,
			Wait:           qdrant.PtrOf(true),
		}); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			v.logger.Warn("Payload index creation failed",
				zap.String("field", idx.field),
				zap.String("collection", collection),
				zap.Error(err))
		}
	}
	return nil
}

func (v *vectorRepositoryImpl) Upsert(ctx context.Context, collection string, chunks []services.CodeChunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("mismatch between chunks (%d) and vectors (%d)", len(chunks), len(vectors))
	}

	var points []*qdrant.PointStruct
	for i, chunk := range chunks {
		id := uuid.NewMD5(uuid.NameSpaceURL, []byte(chunk.ID)).String()

		payload := make(map[string]*qdrant.Value)
		for k, val := range chunk.Metadata {
			switch valC := val.(type) {
			case string:
				payload[k] = qdrant.NewValueString(valC)
			case uint32:
				payload[k] = qdrant.NewValueInt(int64(valC))
			case int:
				payload[k] = qdrant.NewValueInt(int64(valC))
			case uint:
				payload[k] = qdrant.NewValueInt(int64(valC))
			case uint64:
				payload[k] = qdrant.NewValueInt(int64(valC))
			case float64:
				payload[k] = qdrant.NewValueDouble(valC)
			case bool:
				payload[k] = qdrant.NewValueBool(valC)
			default:
				payload[k] = qdrant.NewValueString(fmt.Sprintf("%v", valC))
			}
		}
		payload["content"] = qdrant.NewValueString(chunk.Content)
		payload["chunk_id"] = qdrant.NewValueString(chunk.ID)

		points = append(points, &qdrant.PointStruct{
			Id:      qdrant.NewIDUUID(id),
			Vectors: newDenseVectorsCompat(vectors[i]),
			Payload: payload,
		})
	}

	if len(points) > 0 {
		_, err := v.client.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: collection,
			Wait:           qdrant.PtrOf(true),
			Points:         points,
		})
		return err
	}
	return nil
}

func (v *vectorRepositoryImpl) Search(ctx context.Context, collection string, queryVec []float32, limit int, filter services.SearchFilter) ([]repositories.SearchResult, error) {
	var mustConditions []*qdrant.Condition
	if filter.OrgID != "" {
		mustConditions = append(mustConditions, qdrant.NewMatch("github_org_id", filter.OrgID))
	}
	if filter.WorkspaceID != "" {
		mustConditions = append(mustConditions, qdrant.NewMatch("workspace_id", filter.WorkspaceID))
	}
	if filter.UserID != "" {
		mustConditions = append(mustConditions, qdrant.NewMatch("user_id", filter.UserID))
	}
	if filter.MachineID != "" {
		mustConditions = append(mustConditions, qdrant.NewMatch("machine_id", filter.MachineID))
	}
	if filter.RepoID > 0 {
		mustConditions = append(mustConditions, qdrant.NewMatchInt("repo_id", int64(filter.RepoID)))
	}

	var qdrantFilter *qdrant.Filter
	if len(mustConditions) > 0 {
		qdrantFilter = &qdrant.Filter{
			Must: mustConditions,
		}
	}

	res, err := v.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQuery(queryVec...),
		Filter:         qdrantFilter,
		Limit:          qdrant.PtrOf(uint64(limit)),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		return nil, err
	}

	var results []repositories.SearchResult
	for _, point := range res {
		results = append(results, repositories.SearchResult{
			ChunkID:  payloadString(point.Payload, "chunk_id"),
			Content:  payloadString(point.Payload, "content"),
			FilePath: payloadString(point.Payload, "file_path"),
			Language: payloadString(point.Payload, "language"),
			Score:    point.Score,
		})
	}
	return results, nil
}

func (v *vectorRepositoryImpl) IsEmpty(ctx context.Context, collection string) (bool, error) {
	res, err := v.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collection,
		Limit:          qdrant.PtrOf(uint32(1)),
		WithPayload:    qdrant.NewWithPayload(false),
	})
	if err != nil {
		return false, err
	}
	return len(res) == 0, nil
}

func (v *vectorRepositoryImpl) GetByChunkID(ctx context.Context, collection string, chunkID string) (string, error) {
	res, err := v.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collection,
		Filter: &qdrant.Filter{
			Must: []*qdrant.Condition{
				qdrant.NewMatch("chunk_id", chunkID),
			},
		},
		Limit:       qdrant.PtrOf(uint32(1)),
		WithPayload: qdrant.NewWithPayload(true),
	})
	if err != nil {
		return "", err
	}
	if len(res) == 0 {
		return "", fmt.Errorf("chunk not found: %s", chunkID)
	}
	return payloadString(res[0].Payload, "content"), nil
}

func (v *vectorRepositoryImpl) DeleteByFilePath(ctx context.Context, collection string, filePath string) error {
	_, err := v.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collection,
		Points: qdrant.NewPointsSelectorFilter(&qdrant.Filter{
			Must: []*qdrant.Condition{
				qdrant.NewMatch("file_path", filePath),
			},
		}),
		Wait: qdrant.PtrOf(true),
	})
	return err
}

func (v *vectorRepositoryImpl) DeleteByRepoID(ctx context.Context, collection string, repoID uint) error {
	_, err := v.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collection,
		Points: qdrant.NewPointsSelectorFilter(&qdrant.Filter{
			Must: []*qdrant.Condition{
				qdrant.NewMatchInt("repo_id", int64(repoID)),
			},
		}),
		Wait: qdrant.PtrOf(true),
	})
	return err
}

func (v *vectorRepositoryImpl) DeleteCollection(ctx context.Context, collection string) error {
	return v.client.DeleteCollection(ctx, collection)
}

func (v *vectorRepositoryImpl) ListFilePaths(ctx context.Context, collection string, filter services.SearchFilter) ([]string, error) {
	var offset *qdrant.PointId
	seen := make(map[string]bool)

	for {
		res, err := v.client.Scroll(ctx, &qdrant.ScrollPoints{
			CollectionName: collection,
			Limit:          qdrant.PtrOf(uint32(100)),
			Offset:         offset,
			WithPayload:    qdrant.NewWithPayloadInclude("file_path"),
		})
		if err != nil {
			return nil, fmt.Errorf("list file paths failed: %w", err)
		}
		for _, point := range res {
			fp := payloadString(point.Payload, "file_path")
			if fp != "" {
				seen[fp] = true
			}
			offset = point.Id
		}
		if len(res) < 100 {
			break
		}
	}

	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	return paths, nil
}

func (v *vectorRepositoryImpl) GetChunksByFilePath(ctx context.Context, collection string, filePath string) ([]string, error) {
	res, err := v.client.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collection,
		Limit:          qdrant.PtrOf(uint32(50)),
		Filter: &qdrant.Filter{
			Must: []*qdrant.Condition{
				qdrant.NewMatch("file_path", filePath),
			},
		},
		WithPayload: qdrant.NewWithPayloadInclude("content"),
	})
	if err != nil {
		return nil, fmt.Errorf("get chunks by file path failed: %w", err)
	}

	chunks := make([]string, 0, len(res))
	for _, point := range res {
		content := payloadString(point.Payload, "content")
		if content != "" {
			chunks = append(chunks, content)
		}
	}
	return chunks, nil
}

func payloadString(payload map[string]*qdrant.Value, key string) string {
	if val, ok := payload[key]; ok && val != nil {
		return val.GetStringValue()
	}
	return ""
}

// newDenseVectorsCompat builds a Vectors proto that sets BOTH the deprecated
// Vector.Data field (proto field 1, read by Qdrant <= 1.12) AND the new
// Vector.Dense oneof (proto field 101, read by Qdrant >= 1.13+). This lets
// a single binary work against old and new Qdrant clusters.
//
// Background: go-client v1.17.1's NewVectors() only populates the Dense
// oneof. Qdrant v1.11.3 (our staging server) doesn't understand field 101
// and reads field 1 — which is empty — yielding "expected dim: 3072, got 0".
func newDenseVectorsCompat(vec []float32) *qdrant.Vectors {
	return &qdrant.Vectors{
		VectorsOptions: &qdrant.Vectors_Vector{
			Vector: &qdrant.Vector{
				Data: vec, // deprecated field 1 — old servers read this
				Vector: &qdrant.Vector_Dense{ // new field 101 — new servers read this
					Dense: &qdrant.DenseVector{
						Data: vec,
					},
				},
			},
		},
	}
}
