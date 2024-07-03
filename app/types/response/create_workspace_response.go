package response

type WorkspaceDetails struct {
	WorkspaceId      string  `json:"workspaceId"`
	BackendTemplate  *string `json:"backendTemplate,omitempty"`
	FrontendTemplate *string `json:"frontendTemplate,omitempty"`
	WorkspaceUrl     *string `json:"workspaceUrl,omitempty"`
	FrontendUrl      *string `json:"frontendUrl,omitempty"`
	BackendUrl       *string `json:"backendUrl,omitempty"`
}

type CreateWorkspaceResponse struct {
	Message          string            `json:"message"`
	WorkspaceDetails *WorkspaceDetails `json:"workspace"`
}
