package local_storage_providers

// import (
// 	"ai-developer/app/config"
// 	"bytes"
// 	"encoding/base64"
// 	"fmt"
// 	"image/jpeg"
// 	"image/png"
// 	"net/url"
// 	"os"
// 	"path/filepath"
// )

// type LocalStorageService struct {}

// func NewLocalStorageService() *LocalStorageService { 
// 	return &LocalStorageService{}
// }

// func (service *LocalStorageService) UploadFile(fileBytes []byte, fileName string, projectHashID string, projectID, storyID int) (string, error) {
// 	storageDir := config.WorkspaceWorkingDirectory() + "/" + projectHashID + "/" + ".storage"
// 	if err := os.MkdirAll(storageDir, os.ModePerm); err != nil {
// 		return "", fmt.Errorf("failed to create .storage folder: %w", err)
// 	}
// 	dirPath := filepath.Join(storageDir, fmt.Sprintf("%d/%d", projectID, storyID))
// 	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
// 		return "", fmt.Errorf("failed to create directory: %w", err)
// 	}

// 	filePath := filepath.Join(dirPath, fileName)

// 	if err := os.WriteFile(filePath, fileBytes, os.ModePerm); err != nil {
// 		return "", fmt.Errorf("failed to write file: %w", err)
// 	}

// 	return filePath, nil
// }

// func (service *LocalStorageService) DeleteFile(localURL string) error {
// 	// Parse the local URL
// 	parsedURL, err := url.Parse(localURL)
// 	if err != nil {
// 		return fmt.Errorf("failed to parse local URL: %w", err)
// 	}

// 	// Extract the file path from the URL
// 	filePath := parsedURL.Path

// 	// Delete the file
// 	if err := os.Remove(filePath); err != nil {
// 		return fmt.Errorf("failed to delete file: %w", err)
// 	}

// 	fmt.Println("Successfully deleted file:", localURL)
// 	return nil
// }

// func (service *LocalStorageService) GetBase64FromLocalUrl(localURL string) (string, string, error) {
// 	// Parse the local URL
// 	parsedURL, err := url.Parse(localURL)
// 	if err != nil {
// 		return "", "", fmt.Errorf("failed to parse local URL: %w", err)
// 	}

// 	// Extract the file path from the URL
// 	filePath := parsedURL.Path

// 	// Read the image data from the file
// 	imageData, err := os.ReadFile(filePath)
// 	if err != nil {
// 		return "", "", fmt.Errorf("failed to read image data: %w", err)
// 	}

// 	imageType, err := determineImageType(imageData)
// 	if err != nil {
// 		return "", "", fmt.Errorf("failed to determine image type: %w", err)
// 	}

// 	// Encode the image data to Base64
// 	base64Image := base64.StdEncoding.EncodeToString(imageData)

// 	return base64Image, imageType, nil
// }

// func determineImageType(data []byte) (string, error) {
// 	if _, err := jpeg.DecodeConfig(bytes.NewReader(data)); err == nil {
// 		return "image/jpeg", nil
// 	}
// 	if _, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
// 		return "image/png", nil
// 	}
// 	return "", fmt.Errorf("unsupported image type")
// }
