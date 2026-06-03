package config

import "github.com/knadh/koanf/v2"

type KafkaTopicsConfig interface {
	Cluster() string
	ActionCodeReviewTopic() string
}

type kafkaTopicsConfigImpl struct {
	config *koanf.Koanf
}

func NewKafkaTopicsConfig(config *koanf.Koanf) KafkaTopicsConfig {
	return &kafkaTopicsConfigImpl{config: config}
}

func (c *kafkaTopicsConfigImpl) Cluster() string {
	v := c.config.String("kafka.cluster.name")
	if v == "" {
		return "coder"
	}
	return v
}

func (c *kafkaTopicsConfigImpl) ActionCodeReviewTopic() string {
	v := c.config.String("kafka.topics.action.code_review")
	if v == "" {
		return "superagi.git.actions.code_review"
	}
	return v
}

// Backward-compatible constants (used by consumers/producers that haven't migrated to config yet).
const (
	KafkaCluster          = "coder"
	TopicActionCodeReview = "superagi.git.actions.code_review"
)
