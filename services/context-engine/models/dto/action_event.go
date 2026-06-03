package dto

import "time"

// ActionEvent represents an automation action event consumed from Kafka.
type ActionEvent struct {
	RuleID     uint64                 `json:"rule_id"`
	RuleName   string                 `json:"rule_name"`
	ActionType string                 `json:"action_type"`
	Config     map[string]interface{} `json:"config"`
	GitEvent   *GitEvent              `json:"git_event"`
}

// GitEvent represents a normalized git event from coder-integrations.
type GitEvent struct {
	EventID     string                 `json:"event_id"`
	EventType   string                 `json:"event_type"`
	Provider    string                 `json:"provider"`
	WorkspaceID uint64                 `json:"workspace_id"`
	Repository  string                 `json:"repository"`
	Branch      string                 `json:"branch,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Payload     map[string]interface{} `json:"payload"`
}

// PullRequestEventPayload is the typed payload for PR events embedded in GitEvent.Payload.
type PullRequestEventPayload struct {
	Action string `json:"action"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Head   string `json:"head"`
	Base   string `json:"base"`
	User   string `json:"user"`
	Merged bool   `json:"merged"`
}
