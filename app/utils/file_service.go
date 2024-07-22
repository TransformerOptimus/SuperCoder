package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"log"
)

func WriteToFile(filename string, content []string) error {
	fmt.Println("_________________________________________________________________")
	fmt.Println("Writing to file:", filename)
	dir := filepath.Dir(filename)
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	_, err = file.WriteString(strings.Join(content, "\n"))
	fmt.Println("Content : ", strings.Join(content, "\n"))
	if err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}
	return nil
}

func RemoveFile(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Println("File does not exist", filePath)
		return nil
	}
	if err := os.Remove(filePath); err != nil {
		log.Println("Error removing file: ", filePath, err)
		return err
	}
	return nil
}
