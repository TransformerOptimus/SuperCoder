package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
)

func EncodeToBase64(filePathCloser io.ReadCloser) (string, string, error) {
	defer filePathCloser.Close()

	content, err := io.ReadAll(filePathCloser)
	if err != nil {
		return "", "", fmt.Errorf("failed to read from ReadCloser: %w", err)
	}
	imageType, err := determineImageType(content)
	if err != nil {
		return "", "", fmt.Errorf("failed to determine image type: %w", err)
	}

	// Encode the content to Base64
	base64Content := base64.StdEncoding.EncodeToString(content)

	return base64Content, imageType, nil
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

