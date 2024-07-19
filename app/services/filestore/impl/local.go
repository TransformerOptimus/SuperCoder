package impl

import (
	"ai-developer/app/config"
	"ai-developer/app/services/filestore"
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"go.uber.org/zap"
)

type LocalFileStore struct {
	filestore.FileStore
	baseFolder string
	logger     *zap.Logger
}

func (lfs LocalFileStore) getFilePath(path string) (filePath string, err error) {
	filePath, err = url.JoinPath(lfs.baseFolder, path)
	if err != nil {
		lfs.logger.Error("Failed to get file path", zap.Error(err))
		return
	}
	return
}

func (lfs LocalFileStore) CreateFileFromContent(path string, content []byte) (err error) {
	filePath, err := lfs.getFilePath(path)
	if err != nil {
		return
	}
	// Ensure the directory exists
	dir := filepath.Dir(filePath)
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return
	}
	err = os.WriteFile(filePath, content, 0644)
	if err!= nil {
        fmt.Println("Failed to write file", err)
        return err
    }
	return nil
}

func (lfs LocalFileStore) ReadFile(path string) (content io.ReadCloser, err error) {
	filePath, err := lfs.getFilePath(path)
	if err != nil {
		return
	}
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Println("Failed to read file", err)
		return
	}
	content = io.NopCloser(bytes.NewReader(fileContent))
	return content, nil
}

func (lfs LocalFileStore) DeleteFile(path string) (err error) {
	filePath, err := lfs.getFilePath(path)
	if err != nil {
		return
	}
	err = os.Remove(filePath)
	return nil
}

func NewLocalFileStore(fileStoreConfig *config.FileStoreConfig, logger *zap.Logger) filestore.FileStore {
	return LocalFileStore{
		baseFolder: fileStoreConfig.GetLocalDir(),
		logger:     logger,
	}
}
