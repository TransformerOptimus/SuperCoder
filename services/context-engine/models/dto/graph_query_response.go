package dto

type GraphResultItem struct {
	Name      string `json:"name"`
	FilePath  string `json:"file_path"`
	ChunkID   string `json:"chunk_id"`
	Depth     int    `json:"depth"`
	Direction string `json:"direction,omitempty"` // "caller" (blast_radius) or "dependency"
}

type GraphQueryResponse struct {
	Total    int               `json:"total"`
	Results  []GraphResultItem `json:"results"`
	Indexing bool              `json:"indexing,omitempty"`
	Message  string            `json:"message,omitempty"`
}
