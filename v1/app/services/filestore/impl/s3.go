package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/services/filestore"
	"bytes"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"go.uber.org/zap"
	"io"
	"net/url"
)

type S3FileStore struct {
	filestore.FileStore
	bucket   string
	basePath string
	s3Client *s3.S3
	logger   *zap.Logger
}

func (s3fs S3FileStore) getFilePath(path string) (filePath string, err error) {
	filePath, err = url.JoinPath(s3fs.basePath, path)
	if err != nil {
		s3fs.logger.Error("Failed to get file path", zap.Error(err))
		return
	}
	return
}

func (s3fs S3FileStore) CreateFileFromContent(path string, content []byte) (err error) {
	filePath, err := s3fs.getFilePath(path)
	if err != nil {
		return err
	}
	_, err = s3fs.s3Client.PutObject(&s3.PutObjectInput{
		Key:    aws.String(filePath),
		Bucket: aws.String(s3fs.bucket),
		Body:   bytes.NewReader(content),
	})
	if err != nil {
		s3fs.logger.Error("Failed to put object", zap.Error(err))
		return err
	}
	return nil
}

func (s3fs S3FileStore) ReadFile(path string) (content io.ReadCloser, err error) {
	filePath, err := s3fs.getFilePath(path)
	if err != nil {
		return nil, err
	}
	output, err := s3fs.s3Client.GetObject(&s3.GetObjectInput{
		Key:    aws.String(filePath),
		Bucket: aws.String(s3fs.bucket),
	})
	if err != nil {
		s3fs.logger.Error("Failed to get object", zap.Error(err))
		return nil, err
	}
	content = output.Body
	return content, nil
}

func (s3fs S3FileStore) ReadFileWithInfo(path string) (content io.ReadCloser, contentLength int64, contentType *string, err error) {
    filePath, err := s3fs.getFilePath(path)
    if err != nil {
        return nil, 0, nil, err
    }

    output, err := s3fs.s3Client.GetObject(&s3.GetObjectInput{
        Key:    aws.String(filePath),
        Bucket: aws.String(s3fs.bucket),
    })
    if err != nil {
        s3fs.logger.Error("Failed to get object", zap.Error(err))
        return nil, 0, nil, err
    }

    return output.Body, *output.ContentLength, output.ContentType, nil
}

func (s3fs S3FileStore) DeleteFile(path string) (err error) {
	filePath, err := s3fs.getFilePath(path)
	if err != nil {
		return err
	}
	_, err = s3fs.s3Client.DeleteObject(&s3.DeleteObjectInput{
		Key:    aws.String(filePath),
		Bucket: aws.String(s3fs.bucket),
	})
	if err != nil {
		s3fs.logger.Error("Failed to delete object", zap.Error(err))
		return err
	}
	return nil
}

func NewS3FileSystem(
	awsSession *session.Session,
	s3fileStoreConfig *config.S3FileStoreConfig,
	logger *zap.Logger,
) filestore.FileStore {
	return S3FileStore{
		s3Client: s3.New(awsSession),
		bucket:   s3fileStoreConfig.GetS3Bucket(),
		basePath: s3fileStoreConfig.GetS3Path(),
		logger:   logger,
	}
}
