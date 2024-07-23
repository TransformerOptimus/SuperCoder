package request

type ImportGitRepository struct {
	WorkspaceId string `json:"workspaceId"`

	Repository string `json:"repository"`
	Username   string `json:"username"`
	Password   string `json:"password"`

	RemoteURL    string `json:"remoteURL"`
	GitnessUser  string `json:"gitnessUser"`
	GitnessToken string `json:"gitnessToken"`
}
