package dto

type CreateWorkspace struct {
	WorkspaceId      string  `json:"workspaceId"`
	RemoteURL        string  `json:"remoteURL"`
	FrontendTemplate *string `json:"frontendTemplate,omitempty"`
	BackendTemplate  *string `json:"backendTemplate,omitempty"`
	GitnessUserName  string  `json:"gitnessUserName"`
	GitnessToken     string  `json:"gitnessToken"`
}
