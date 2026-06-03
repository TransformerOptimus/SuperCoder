package dto

type SearchResultItem struct {
	ChunkID  string  `json:"chunk_id"`
	Content  string  `json:"content"`
	FilePath string  `json:"file_path"`
	Language string  `json:"language"`
	Score    float32 `json:"score"`
	Source   string  `json:"source"`
}

type SearchResponse struct {
	Query   string             `json:"query"`
	Total   int                `json:"total"`
	Results []SearchResultItem `json:"results"`
}
