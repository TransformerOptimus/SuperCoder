package filestore

import "io"

type FileStore interface {
	CreateFileFromContent(path string, content []byte) (err error)
	ReadFile(path string) (content io.ReadCloser, err error)
	DeleteFile(path string) (err error)
}
