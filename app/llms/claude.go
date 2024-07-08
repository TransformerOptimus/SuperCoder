package llms

import (
	"ai-developer/app/client"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type ClaudeClient struct {
	Model            string
	Temperature      float64
	MaxTokens        int
	TopP             float64
	FrequencyPenalty float64
	PresencePenalty  float64
	NumberOfResults  int
	ApiKey           string
	ResponseFormat   *ClaudeResponseFormat
	HttpClient       *client.HttpClient
	ApiBaseUrl       string
	RetryAttempts    int
	BackoffFactor    int
}

type ClaudeResponseFormat struct {
	Type string `json:"type"`
}

type ClaudeChatCompletionMessage struct {
	Role    string           `json:"role"`
	Content []MessageContent `json:"content"`
}

type MessageContent struct {
	Type   string           `json:"type"`
	Text   string           `json:"text,omitempty"`
	Source *ImageSourceData `json:"source,omitempty"`
}

type ImageSourceData struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ClaudeChatCompletionRequest struct {
	Model       string                        `json:"model"`
	Messages    []ClaudeChatCompletionMessage `json:"messages"`
	Temperature float64                       `json:"temperature"`
	MaxTokens   int                           `json:"max_tokens"`
}

type ClaudeChatCompletionResponseMessage struct {
	ID           string      `json:"id"`
	Type         string      `json:"type"`
	Role         string      `json:"role"`
	Model        string      `json:"model"`
	Content      []TextBlock `json:"content"`
	StopReason   string      `json:"stop_reason"`
	StopSequence interface{} `json:"stop_sequence"`
	Usage        Usage       `json:"usage"`
}

type TextBlock struct {
	Text string `json:"text"`
	Type string `json:"type"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func NewClaudeClient(apiKey string) *ClaudeClient {
	apiBaseUrl := os.Getenv("CLAUDE_API_BASE")
	if apiBaseUrl == "" {
		apiBaseUrl = "https://api.anthropic.com/v1"
	}

	return &ClaudeClient{
		Model:            "claude-3-5-sonnet-20240620",
		Temperature:      1.0,
		MaxTokens:        4000,
		TopP:             1,
		ApiKey:           apiKey,
		FrequencyPenalty: 0,
		PresencePenalty:  0,
		NumberOfResults:  1,
		HttpClient:       client.NewHttpClient(),
		RetryAttempts:    3,
		BackoffFactor:    2,
		ResponseFormat:   &ClaudeResponseFormat{Type: "text"},
		ApiBaseUrl:       apiBaseUrl,
	}
}

func (c *ClaudeClient) WithApiKey(apiKey string) {
	c.ApiKey = apiKey
}

func (c *ClaudeClient) ChatCompletion(messages []ClaudeChatCompletionMessage) (string, error) {
	url := fmt.Sprintf("%s/messages", c.ApiBaseUrl)
	fmt.Println("Model Name", c.Model)
	fmt.Println("___INPUT____", messages)
	requestBody := ClaudeChatCompletionRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: c.Temperature,
		MaxTokens:   c.MaxTokens,
	}

	headers := map[string]string{
		"content-type":      "application/json",
		"x-api-key":         c.ApiKey,
		"anthropic-version": "2023-06-01",
	}

	response, err := c.HttpClient.Post(url, requestBody, headers)
	body, _ := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get response from Claude API, status code: %d", response.StatusCode)
	}

	var chatResponse ClaudeChatCompletionResponseMessage
	if err := json.Unmarshal(body, &chatResponse); err != nil {
		log.Fatalf("Error unmarshalling JSON: %v", err)
	}

	if len(chatResponse.Content) == 0 {
		fmt.Println("Error fetching choices")
		return "", fmt.Errorf("no response choices found")
	}

	fmt.Println("___OUTPUT____: ", chatResponse.Content[0].Text)
	return chatResponse.Content[0].Text, nil
}
