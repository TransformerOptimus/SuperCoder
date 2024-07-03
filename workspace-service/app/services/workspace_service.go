package services

import "workspace-service/app/models/dto"

type WorkspaceService interface {
	CreateWorkspace(workspaceId string, backendTemplate string, remoteURL string) (*dto.WorkspaceDetails, error)
	DeleteWorkspace(workspaceId string) error
}
