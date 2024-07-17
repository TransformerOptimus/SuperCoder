package response

type SendEmailResponse struct {
	Success   bool   `json:"success"`
	MessageId string `json:"message_id"`
	Error     string `json:"error"`
}
