package email

import (
	"ai-developer/app/types/request"
	"ai-developer/app/types/response"
)

type EmailService interface {
	SendOutboundEmail(request *request.SendEmailRequest) (*response.SendEmailResponse, error)
}
