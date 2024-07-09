package s3_providers

import (
	"ai-developer/app/config"
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"image/jpeg"
	"image/png"
	"io"
	"net/url"
	"strings"
)

type S3Service struct {
	awsAccessKeyID     string
	awsSecretAccessKey string
	awsRegion          string
	bucketName         string
	session            *session.Session
}

func (service *S3Service) UploadFileToS3(fileBytes []byte, fileName string, projectID, storyID int) (string, error) {
	s3Client := s3.New(service.session)

	// Create object key
	objectKey := fmt.Sprintf("%d/%d/%s", projectID, storyID, fileName)

	_, err := s3Client.PutObject(&s3.PutObjectInput{
		Bucket:      aws.String(service.bucketName),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(fileBytes),
		ContentType: aws.String("application/octet-stream"), // You may want to determine the correct MIME type
		ACL:         aws.String("public-read"),              // Make the object publicly accessible
	})
	if err != nil {
		return "", err
	}

	// Create an S3 link
	s3Link := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", service.bucketName, objectKey)

	return s3Link, nil
}

func (service *S3Service) DeleteS3Object(s3URL string) error {
	// Parse the S3 URL
	parsedURL, err := url.Parse(s3URL)
	if err != nil {
		return fmt.Errorf("failed to parse S3 URL: %w", err)
	}

	// Extract the bucket name and object key from the URL
	bucket := service.bucketName
	key := strings.TrimPrefix(parsedURL.Path, "/")

	// Create an S3 client
	s3Client := s3.New(service.session)

	// Delete the object
	_, err = s3Client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	// Wait until the object is deleted
	err = s3Client.WaitUntilObjectNotExists(&s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to wait for object deletion: %w", err)
	}

	fmt.Println("Successfully deleted object:", s3URL)
	return nil
}

func (service *S3Service) GetBase64FromS3Url(s3URL string) (string, string, error) {
	parsedURL, err := url.Parse(s3URL)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse S3 URL: %w", err)
	}

	// Extract the bucket name and object key from the URL
	bucket := service.bucketName
	key := strings.TrimPrefix(parsedURL.Path, "/")

	// Create an S3 client
	s3Client := s3.New(service.session)
	getObjectOutput, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to get object from S3, %v", err)
	}
	defer getObjectOutput.Body.Close()

	// Read the image data into memory
	imageData, err := io.ReadAll(getObjectOutput.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read image data, %v", err)
	}

	imageType, err := determineImageType(imageData)
	if err != nil {
		return "", "", fmt.Errorf("failed to determine image type: %v", err)
	}

	// Encode the image data to Base64
	base64Image := base64.StdEncoding.EncodeToString(imageData)

	return base64Image, imageType, nil
}

func determineImageType(data []byte) (string, error) {
	if _, err := jpeg.DecodeConfig(bytes.NewReader(data)); err == nil {
		return "image/jpeg", nil
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
		return "image/png", nil
	}
	return "", fmt.Errorf("unsupported image type")
}

func NewS3Service() (*S3Service, error) {
	awsAccessKeyID := config.AWSAccessKeyID()
	awsSecretAccessKey := config.AWSSecretAccessKey()
	awsBucketName := config.AWSBucketName()
	awsRegion := config.AWSRegion()
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion),
		Credentials: credentials.NewStaticCredentials(
			awsAccessKeyID,
			awsSecretAccessKey,
			""),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &S3Service{
		awsAccessKeyID:     awsAccessKeyID,
		awsSecretAccessKey: awsSecretAccessKey,
		awsRegion:          awsRegion,
		bucketName:         awsBucketName,
		session:            sess,
	}, nil
}
