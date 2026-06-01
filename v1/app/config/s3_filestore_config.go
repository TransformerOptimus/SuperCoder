package config

import (
	"github.com/knadh/koanf/v2"
)

type S3FileStoreConfig struct {
	config *koanf.Koanf
}

func (fsc *S3FileStoreConfig) GetS3Bucket() string {
	return fsc.config.String("filestore.s3.bucket")
}

func (fsc *S3FileStoreConfig) GetS3Path() string {
	return fsc.config.String("filestore.s3.path")
}

func NewS3FileStoreConfig(config *koanf.Koanf) *S3FileStoreConfig {
	return &S3FileStoreConfig{config}
}
