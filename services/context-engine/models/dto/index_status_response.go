package dto

type IndexStatusResponse struct {
	Exists         bool   `json:"exists"`
	CollectionName string `json:"collection_name"`
	RepoID         uint   `json:"repo_id"`
	Empty          bool   `json:"empty"`
}

type IndexDeleteRequest struct {
	RepoPath    string `json:"repo_path" binding:"required"`
	UserID      string `json:"user_id"`
	WorkspaceID uint64 `json:"workspace_id"`
	MachineID   string `json:"machine_id"`
}

type IndexDeleteResponse struct {
	Deleted bool   `json:"deleted"`
	Message string `json:"message"`
}
