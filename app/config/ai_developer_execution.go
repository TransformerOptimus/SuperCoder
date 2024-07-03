package config

import "github.com/knadh/koanf/v2"

type AIDeveloperExecutionConfig struct {
	config *koanf.Koanf
}

func (adec *AIDeveloperExecutionConfig) GetStoryID() int64 {
	return config.MustInt64("execution.story.id")
}
func (adec *AIDeveloperExecutionConfig) IsReExecution() bool {
	return config.Bool("execution.reexecution")
}
func (adec *AIDeveloperExecutionConfig) GetBranch() string {
	return config.String("execution.branch")
}
func (adec *AIDeveloperExecutionConfig) GetPullRequestID() int64 {
	return config.Int64("execution.pullrequest.id")
}
func (adec *AIDeveloperExecutionConfig) GetExecutionID() int64 {
	return config.Int64("execution.id")
}

func NewAIDeveloperExecutionConfig(config *koanf.Koanf) *AIDeveloperExecutionConfig {
	return &AIDeveloperExecutionConfig{config}
}
