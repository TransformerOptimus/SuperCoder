package request

type SendEmailRequest struct {
	ToEmail     string
	Subject     string
	Content     string
	HtmlContent string
}
