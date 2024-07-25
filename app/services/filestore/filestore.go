package filestore

import "io"

type FileStore interface {
	CreateFileFromContent(path string, content []byte) (err error)
	ReadFile(path string) (content io.ReadCloser, err error)
	ReadFileWithInfo(path string) (content io.ReadCloser, contentLength int64, contentType *string, err error)
	DeleteFile(path string) (err error)
}
