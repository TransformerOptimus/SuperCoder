package impl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"

	"github.com/TransformerOptimus/SuperCoder/services/context-engine/config"
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/services"
)

type s3SourceProvider struct {
	client *s3.Client
	bucket string
	prefix string
	logger *zap.Logger
}

// NewS3SourceProvider creates a SourceProvider backed by an S3 bucket using global config.
func NewS3SourceProvider(cfg config.IndexerConfig, logger *zap.Logger) (services.SourceProvider, error) {
	return NewS3SourceProviderWithParams(cfg.S3Bucket(), cfg.S3Prefix(), cfg.S3Region(), cfg.S3Endpoint(), logger)
}

// NewS3SourceProviderWithParams creates a SourceProvider backed by an S3 bucket
// using explicit parameters instead of config. This allows per-request S3 routing.
// endpoint can be empty for real AWS S3, or a URL like "http://minio:9000" for S3-compatible stores.
func NewS3SourceProviderWithParams(bucket, prefix, region, endpoint string, logger *zap.Logger) (services.SourceProvider, error) {
	if region == "" {
		region = "us-east-1"
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = true
		})
	}

	return &s3SourceProvider{
		client: s3.NewFromConfig(awsCfg, s3Opts...),
		bucket: bucket,
		prefix: prefix,
		logger: logger.Named("source-s3"),
	}, nil
}

func (p *s3SourceProvider) ListFiles(ctx context.Context, root string) ([]services.FileInfo, error) {
	prefix := p.prefix
	if root != "" && root != "/" {
		prefix = root
	}
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}

	paginator := s3.NewListObjectsV2Paginator(p.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(p.bucket),
		Prefix: aws.String(prefix),
	})

	var files []services.FileInfo
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3 list objects failed: %w", err)
		}
		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			relPath := key
			if prefix != "" {
				relPath = key[len(prefix):]
			}
			if relPath == "" {
				continue
			}
			files = append(files, services.FileInfo{
				Path:         key,
				RelativePath: relPath,
				Size:         aws.ToInt64(obj.Size),
				IsDir:        false,
			})
		}
	}
	return files, nil
}

func (p *s3SourceProvider) ReadFile(ctx context.Context, path string) ([]byte, error) {
	// If the path doesn't start with the configured prefix, prepend it.
	// This handles the case where callers pass relative paths from the merkle tree.
	key := path
	if p.prefix != "" && !strings.HasPrefix(path, p.prefix) {
		sep := ""
		if !strings.HasSuffix(p.prefix, "/") && !strings.HasPrefix(path, "/") {
			sep = "/"
		}
		key = p.prefix + sep + path
	}
	out, err := p.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get object %s failed: %w", path, err)
	}
	defer out.Body.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, out.Body); err != nil {
		return nil, fmt.Errorf("s3 read body %s failed: %w", path, err)
	}
	return buf.Bytes(), nil
}

func (p *s3SourceProvider) GetFileHash(ctx context.Context, path string) (string, error) {
	data, err := p.ReadFile(ctx, path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

func (p *s3SourceProvider) Type() string { return "s3" }
func (p *s3SourceProvider) Close() error { return nil }
