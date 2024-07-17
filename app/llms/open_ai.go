package llms

import (
	"ai-developer/app/client"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type OpenAiClient struct {
	ApiKey           string
	Model            string
	Temperature      float64
	MaxTokens        int
	TopP             float64
	FrequencyPenalty float64
	PresencePenalty  float64
	NumberOfResults  int
	HttpClient       *client.HttpClient
	ApiBaseUrl       string
}

type OpenAiChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAiOpenAiChatCompletionRequest struct {
	Model            string                        `json:"model"`
	Messages         []OpenAiChatCompletionMessage `json:"messages"`
	Temperature      float64                       `json:"temperature"`
	MaxTokens        int                           `json:"max_tokens"`
	TopP             float64                       `json:"top_p"`
	FrequencyPenalty float64                       `json:"frequency_penalty"`
	PresencePenalty  float64                       `json:"presence_penalty"`
	N                int                           `json:"n"`
}

type OpenAiChatCompletionChoice struct {
	Message OpenAiChatCompletionMessage `json:"message"`
}

type OpenAiChatCompletionResponse struct {
	Choices []OpenAiChatCompletionChoice `json:"choices"`
}

func NewOpenAiClient(apiKey string) *OpenAiClient {
	apiBaseUrl := os.Getenv("OPENAI_API_BASE")
	if apiBaseUrl == "" {
		apiBaseUrl = "https://api.openai.com/v1"
	}

	return &OpenAiClient{
		ApiKey:           apiKey,
		Model:            "gpt-4o",
		Temperature:      0.1,
		MaxTokens:        4000,
		TopP:             1,
		FrequencyPenalty: 0,
		PresencePenalty:  0,
		NumberOfResults:  1,
		HttpClient:       client.NewHttpClient(),
		ApiBaseUrl:       apiBaseUrl,
	}
}

func (c *OpenAiClient) WithApiKey(apiKey string) {
	c.ApiKey = apiKey
}

func (c *OpenAiClient) ChatCompletion(messages []OpenAiChatCompletionMessage) (string, error) {
	url := fmt.Sprintf("%s/chat/completions", c.ApiBaseUrl)

	requestBody := OpenAiOpenAiChatCompletionRequest{
		Model:            c.Model,
		Messages:         messages,
		Temperature:      c.Temperature,
		MaxTokens:        c.MaxTokens,
		TopP:             c.TopP,
		FrequencyPenalty: c.FrequencyPenalty,
		PresencePenalty:  c.PresencePenalty,
		N:                c.NumberOfResults,
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + c.ApiKey,
	}

	response, err := c.HttpClient.Post(url, requestBody, headers)
	fmt.Println("Response: ", response)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get response from OpenAI API, status code: %d", response.StatusCode)
	}

	var chatResponse OpenAiChatCompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&chatResponse); err != nil {
		return "", err
	}

	if len(chatResponse.Choices) == 0 {
		return "", fmt.Errorf("no response choices found")
	}

	return chatResponse.Choices[0].Message.Content, nil
}
