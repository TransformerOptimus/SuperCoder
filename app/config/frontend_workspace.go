package config

import "fmt"

func FrontendWorkspacePath(projectHashID string, storyHashId string) string{
	output := config.String("frontend.base.path") + projectHashID + "/" + storyHashId
	fmt.Println("___frontend workspace____",output)
	return output
}