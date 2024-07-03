package response

type GetAllCommitsResponse struct {
	Title    string `json:"title"`
	Commiter string `json:"commiter"`
	SHA      string `json:"sha"`
	Time     string `json:"time"`
	Date     string `json:"date"`
}
