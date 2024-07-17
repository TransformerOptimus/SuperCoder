package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type DirectoryStructure struct {
	Name    string               `json:"name"`
	Folders []DirectoryStructure `json:"folders"`
	Files   []string             `json:"files"`
}

func GetDirectoryStructure(storyDir string) (string, error) {
	directoryStructure := DirectoryStructure{
		Name:    filepath.Base(storyDir),
		Folders: []DirectoryStructure{},
		Files:   []string{},
	}

	dirMap := make(map[string]*DirectoryStructure)
	dirMap[storyDir] = &directoryStructure

	err := filepath.Walk(storyDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == storyDir {
			return nil
		}

		parentDir := filepath.Dir(path)
		parent, ok := dirMap[parentDir]
		if !ok {
			return fmt.Errorf("parent directory not found: %s", parentDir)
		}

		if info.IsDir() {
			if info.Name() == "node_modules" || info.Name() == ".next" {
				return filepath.SkipDir
			}

			subDir := DirectoryStructure{
				Name:    info.Name(),
				Folders: []DirectoryStructure{},
				Files:   []string{},
			}

			parent.Folders = append(parent.Folders, subDir)
			dirMap[path] = &parent.Folders[len(parent.Folders)-1]
		} else {
			parent.Files = append(parent.Files, info.Name())
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	planJSON, err := json.MarshalIndent(directoryStructure, "", "  ")
	if err != nil {
		return "", err
	}

	return string(planJSON), nil

}
