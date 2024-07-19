package services

import "workspace-service/app/models/dto"

type WorkspaceService interface {
	ImportGitRepository(
		workspaceId string,
		repository string,
		username string,
		password string,
		remoteURL string,
		gitnessUser string,
		gitnessToken string,
	) (*dto.WorkspaceDetails, error)
	CreateWorkspace(workspaceId string, backendTemplate string, frontendTemplate *string, remoteURL string, gitnessUser string, gitnessToken string) (*dto.WorkspaceDetails, error)
	CreateFrontendWorkspace(storyHashId, workspaceId string, frontendTemplate string) (*dto.WorkspaceDetails, error)
	DeleteWorkspace(workspaceId string) error
}
