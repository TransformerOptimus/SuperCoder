package dto

type CreateWorkspace struct {
	StoryHashId      string  `json:"storyHashId"`
	WorkspaceId      string  `json:"workspaceId"`
	RemoteURL        string  `json:"remoteURL"`
	FrontendTemplate *string `json:"frontendTemplate,omitempty"`
	BackendTemplate  *string `json:"backendTemplate,omitempty"`
}
