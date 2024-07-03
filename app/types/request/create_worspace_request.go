package request

type CreateWorkspaceRequest struct {
	WorkspaceId      string  `json:"workspaceId"`
	RemoteURL        string  `json:"remoteURL"`
	BackendTemplate  *string `json:"backendTemplate,omitempty"`
	FrontendTemplate *string `json:"frontendTemplate,omitempty"`
}

func (receiver *CreateWorkspaceRequest) WithBackendTemplate(backendTemplate string) *CreateWorkspaceRequest {
	receiver.BackendTemplate = &backendTemplate
	return receiver
}

func (receiver *CreateWorkspaceRequest) WithFrontendTemplate(frontendTemplate string) *CreateWorkspaceRequest {
	receiver.FrontendTemplate = &frontendTemplate
	return receiver
}

func NewCreateWorkspaceRequest(workspaceId string) *CreateWorkspaceRequest {
	return &CreateWorkspaceRequest{
		WorkspaceId: workspaceId,
	}
}
