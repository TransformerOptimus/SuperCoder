package impl

import (
	"time"

	"resty.dev/v3"
)

func newRestyClient(apiKey string) *resty.Client {
	return resty.New().
		SetTimeout(60*time.Second).
		SetRetryCount(2).
		SetRetryWaitTime(2*time.Second).
		SetHeader("Authorization", "Bearer "+apiKey).
		SetHeader("Content-Type", "application/json")
}
