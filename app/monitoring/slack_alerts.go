package monitoring

import (
	"ai-developer/app/config"
	"ai-developer/app/constants"
	"bytes"
	"fmt"
	"github.com/goccy/go-json"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
)

type SlackAlert struct {
	WebhookURL string
}

func NewSlackAlert() *SlackAlert {
	webhookURL := config.SlackWebhookURL()
	return &SlackAlert{WebhookURL: webhookURL}
}

func (s *SlackAlert) SendAlert(errorMessage string, metadata map[string]string) error {
	env := config.Get("app.env")
	if env != constants.Production {
		fmt.Println("Skipping Slack alert in non-production environment")
		return nil
	}
	webhookURL := s.WebhookURL
	mentionStr := "<!here>"

	traceback := string(debug.Stack())
	metadata["traceback"] = traceback

	var metadataStr strings.Builder
	for key, value := range metadata {
		metadataStr.WriteString(fmt.Sprintf("- *%s:* %s\n", key, value))
	}

	message := fmt.Sprintf("%s\n*Service:* SUPER_CODER_BACKEND \n*Environment:* %s\n*Error:* %s\n*Details:*\n%s",
		mentionStr, env, errorMessage, metadataStr.String())

	slackData := map[string]string{"text": message}
	jsonData, err := json.Marshal(slackData)
	if err != nil {
		log.Printf("Failed to marshal Slack message: %v", err)
		return err
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to send alert to Slack: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Request to Slack returned an error %d: %s", resp.StatusCode, resp.Status)
	}
	return nil
}
