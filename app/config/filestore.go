package config

import (
	"github.com/knadh/koanf/v2"
)

type FileStoreConfig struct {
	config *koanf.Koanf
}

func (fsc *FileStoreConfig) GetFileStoreType() string {
	return fsc.config.String("filestore.type")
}

func NewFileStoreConfig(config *koanf.Koanf) *FileStoreConfig {
	return &FileStoreConfig{config}
}
