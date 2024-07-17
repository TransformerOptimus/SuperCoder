package response

type GetCodeForDesignStory struct {
	FileName string `json:"file_name"`
	Code     string `json:"code"`
}
