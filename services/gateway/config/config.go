package config

import (
	"fmt"
	"strings"

	"github.com/knadh/koanf/v2"
)

// ModelEntry represents a model available through the gateway.
type ModelEntry struct {
	ID             string
	DisplayName    string
	Provider       string
	ContextWindow  int
	SupportsImages bool
}

// GatewayConfig provides gateway configuration via Koanf.
type GatewayConfig interface {
	DefaultProvider() string
	Providers() []ProviderEntry
	Models() []ModelEntry
}

// ProviderEntry represents a configured LLM provider.
type ProviderEntry struct {
	Name             string
	Adapter          string
	BaseURL          string
	Models           []string
	Version          string
	DefaultMaxTokens int
	APIKey           string
}

type gatewayConfigImpl struct {
	config *koanf.Koanf
}

func NewGatewayConfig(config *koanf.Koanf) GatewayConfig {
	return &gatewayConfigImpl{config: config}
}

func (c *gatewayConfigImpl) DefaultProvider() string {
	return c.config.String("gateway.defaults.provider")
}

func (c *gatewayConfigImpl) Models() []ModelEntry {
	raw := c.config.Get("gateway.models")
	if raw == nil {
		return nil
	}

	items, ok := raw.([]map[string]any)
	if !ok {
		// Try []any (koanf sometimes returns this type)
		rawSlice, ok2 := raw.([]any)
		if !ok2 {
			return nil
		}
		var entries []ModelEntry
		for _, item := range rawSlice {
			if m, ok := item.(map[string]any); ok {
				entries = append(entries, parseModelEntry(m))
			}
		}
		return entries
	}

	entries := make([]ModelEntry, 0, len(items))
	for _, m := range items {
		entries = append(entries, parseModelEntry(m))
	}
	return entries
}

func parseModelEntry(m map[string]any) ModelEntry {
	entry := ModelEntry{}
	if v, ok := m["id"].(string); ok {
		entry.ID = v
	}
	if v, ok := m["provider"].(string); ok {
		entry.Provider = v
	}
	if display, ok := m["display"].(map[string]any); ok {
		if v, ok := display["name"].(string); ok {
			entry.DisplayName = v
		}
	}
	if ctx, ok := m["context"].(map[string]any); ok {
		if v, ok := ctx["window"].(int); ok {
			entry.ContextWindow = v
		}
	}
	if v, ok := m["supports_images"].(bool); ok {
		entry.SupportsImages = v
	}
	return entry
}

func (c *gatewayConfigImpl) Providers() []ProviderEntry {
	var entries []ProviderEntry

	// Extract unique provider names from flat koanf keys like "gateway.providers.anthropic.adapter"
	const prefix = "gateway.providers."
	seen := make(map[string]bool)
	for _, key := range c.config.Keys() {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		rest := strings.TrimPrefix(key, prefix)
		name, _, _ := strings.Cut(rest, ".")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true

		p := fmt.Sprintf("gateway.providers.%s", name)
		entry := ProviderEntry{
			Name:             name,
			Adapter:          c.config.String(p + ".adapter"),
			BaseURL:          c.config.String(p + ".base.url"),
			Models:           c.config.Strings(p + ".models"),
			Version:          c.config.String(p + ".version"),
			DefaultMaxTokens: c.config.Int(p + ".default.max.tokens"),
			APIKey:           c.config.String(p + ".api.key"),
		}
		entries = append(entries, entry)
	}
	return entries
}
