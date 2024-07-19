package config

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/knadh/koanf/v2"
)

type AWSConfig struct {
	config *koanf.Koanf
}

func (ac *AWSConfig) AccessKeyID() *string {
	accessKeyId := ac.config.String("aws.access.key.id")
	if accessKeyId == "" {
		return nil
	} else {
		fmt.Println("____Access Key ID____", accessKeyId)
		return &accessKeyId
	}
}

func (ac *AWSConfig) SecretAccessKey() *string {
	secretAccessKey := ac.config.String("aws.secret.access.key")
	if secretAccessKey == "" {
		return nil
	} else {
		fmt.Println("____secret access key____", secretAccessKey)
		return &secretAccessKey
	}
}

func (ac *AWSConfig) Region() string {
	fmt.Println("_____aws region____", ac.config.String("aws.region"))
	return ac.config.String("aws.region")
}

func NewAWSConfig(config *koanf.Koanf) *AWSConfig {
	return &AWSConfig{
		config: config,
	}
}

func NewAwsSession(awsConfig *AWSConfig) *session.Session {
	config := aws.Config{
		Region: aws.String(awsConfig.Region()),
	}
	if awsConfig.AccessKeyID() == nil && awsConfig.SecretAccessKey() == nil {
		config.Credentials = credentials.NewEnvCredentials()
	} else {
		config.Credentials = credentials.NewStaticCredentials(*awsConfig.AccessKeyID(), *awsConfig.SecretAccessKey(), "")
	}
	return session.Must(session.NewSession(&config))
}
