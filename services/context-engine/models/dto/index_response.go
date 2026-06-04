package dto

type IndexResponse struct {
	Status       string       `json:"status"` // queued, in_progress, completed, failed
	IndexMetrics IndexMetrics `json:"metrics,omitempty"`
}

type IndexMetrics struct {
	FilesProcessed int `json:"files_processed"`
	ChunksCreated  int `json:"chunks_created"`
	GraphNodes     int `json:"graph_nodes"`
}
