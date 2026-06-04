package impl

import (
	"github.com/TransformerOptimus/SuperCoder/services/context-engine/prompts"
)

// PromptProvider provides prompt templates via compile-time embedded text files.
type PromptProvider struct {
	systemReview string
	audit        string
	maxTurns     string
}

func NewPromptProvider() *PromptProvider {
	return &PromptProvider{
		systemReview: prompts.SystemReview,
		audit:        prompts.Audit,
		maxTurns:     prompts.MaxTurns,
	}
}

func (p *PromptProvider) SystemReview() string { return p.systemReview }
func (p *PromptProvider) Audit() string        { return p.audit }
func (p *PromptProvider) MaxTurns() string     { return p.maxTurns }
