package config

import "fmt"
import "path/filepath"

func FrontendWorkspacePath(projectHashID string, storyHashId string) string{
	output := filepath.Join(WorkspaceWorkingDirectory(), projectHashID, "frontend" , ".stories" , storyHashId)
	fmt.Println("___frontend workspace____",output)
	return output
}