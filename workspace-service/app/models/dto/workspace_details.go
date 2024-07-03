package dto

type WorkspaceDetails struct {
	WorkspaceId      string  `json:"workspaceId"`
	BackendTemplate  *string `json:"backendTemplate,omitempty"`
	FrontendTemplate *string `json:"frontendTemplate,omitempty"`
	WorkspaceUrl     *string `json:"workspaceUrl,omitempty"`
	FrontendUrl      *string `json:"frontendUrl,omitempty"`
	BackendUrl       *string `json:"backendUrl,omitempty"`
}
