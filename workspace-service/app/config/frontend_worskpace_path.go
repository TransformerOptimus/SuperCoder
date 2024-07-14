package config

import "fmt"
import "path/filepath"

func FrontendWorkspacePath(projectHashID string, storyHashId string) string{
	output := filepath.Join("/workspaces", projectHashID, "frontend" , ".stories" , storyHashId)
	fmt.Println("___frontend workspace service____",output)
	return output
}