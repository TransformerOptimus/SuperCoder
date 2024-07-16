package utils

import (
	"bytes"
	"mime/multipart"
)

func ReadFileToBytes(file multipart.File) ([]byte, error) {
	fileBytes := bytes.NewBuffer(nil)
	if _, err := fileBytes.ReadFrom(file); err != nil {
		return nil, err
	}
	return fileBytes.Bytes(), nil
}
