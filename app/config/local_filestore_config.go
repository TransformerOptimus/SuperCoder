package config

import (
	"github.com/knadh/koanf/v2"
)

type LocalFileStoreConfig struct {
	config *koanf.Koanf
}

func (lfs *LocalFileStoreConfig) GetLocalDir() string {
	return lfs.config.String("filestore.local.dir")
}

func NewLocalFileStoreConfig(config *koanf.Koanf) *LocalFileStoreConfig {
	return &LocalFileStoreConfig{config}
}
