package config

import (
	"fmt"

	"github.com/knadh/koanf/v2"
)

type FileStoreConfig struct {
	config *koanf.Koanf
}

func (fsc *FileStoreConfig) GetFileStoreType() string {
	return fsc.config.String("filestore.type")
}

func (fsc *FileStoreConfig) GetLocalDir() string {
	return fsc.config.String("filestore.local.dir")
}

func (fsc *FileStoreConfig) GetS3Bucket() string {
	fmt.Println("__BUCKET NAME____",fsc.config.String("filestore.s3.bucket"))
	return fsc.config.String("filestore.s3.bucket")
}

func (fsc *FileStoreConfig) GetS3Path() string {
	return fsc.config.String("filestore.s3.path")
}

func NewFileStoreConfig(config *koanf.Koanf) *FileStoreConfig {
	return &FileStoreConfig{config}
}
