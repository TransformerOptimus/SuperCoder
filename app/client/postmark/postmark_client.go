package postmark

import (
	"ai-developer/app/client"
	"ai-developer/app/config"
	"ai-developer/app/services/email"
	"ai-developer/app/types/request"
	"ai-developer/app/types/response"
	"encoding/json"
	"fmt"
)

type PostmarkClient struct {
	baseURL     string
	httpClient  *client.HttpClient
	fromEmail   string
	serverToken string
}

func (c *PostmarkClient) SendOutboundEmail(sendEmailRequest *request.SendEmailRequest) (*response.SendEmailResponse, error) {
	url := fmt.Sprintf("%s/email", c.baseURL)
	headers := map[string]string{
		"Accept":                  "*/*",
		"Content-Type":            "application/json",
		"X-Postmark-Server-Token": c.serverToken,
	}
	var emailRequest = &request.PostmarkSendEmailRequest{
		From:          c.fromEmail,
		To:            sendEmailRequest.ToEmail,
		Subject:       sendEmailRequest.Subject,
		TextBody:      sendEmailRequest.Content,
		HtmlBody:      sendEmailRequest.HtmlContent,
		MessageStream: "outbound",
	}
	res, err := c.httpClient.Post(url, emailRequest, headers)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var emailResponse *response.PostmarkEmailResponse
	if err := json.NewDecoder(res.Body).Decode(&emailResponse); err != nil {
		return nil, err
	}
	if emailResponse.ErrorCode == 0 {
		return &response.SendEmailResponse{
			Success:   true,
			MessageId: emailResponse.MessageId,
			Error:     "",
		}, nil
	}

	return &response.SendEmailResponse{
		Success:   false,
		MessageId: "",
		Error:     emailResponse.Message,
	}, nil
}

func NewPostmarkClient(
	httpClient *client.HttpClient,
) email.EmailService {
	return &PostmarkClient{
		baseURL:     config.PostmarkBaseURL(),
		httpClient:  httpClient,
		fromEmail:   config.PostmarkFromEmail(),
		serverToken: config.PostmarkOutboundServerToken(),
	}
}
