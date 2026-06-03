package dto

type ContextResponse struct {
	Query    string `json:"query"`
	Context  string `json:"context"`
	Indexing bool   `json:"indexing,omitempty"`
	Message  string `json:"message,omitempty"`
}
