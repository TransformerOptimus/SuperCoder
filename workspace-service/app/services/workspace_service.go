package services

import "workspace-service/app/models/dto"

type WorkspaceService interface {
	CreateWorkspace(workspaceId string, backendTemplate string, frontendTemplate *string, remoteURL string, gitnessUser string, gitnessToken string) (*dto.WorkspaceDetails, error)
	CreateFrontendWorkspace(storyHashId, workspaceId string, frontendTemplate string) (*dto.WorkspaceDetails, error)
	DeleteWorkspace(workspaceId string) error
}
