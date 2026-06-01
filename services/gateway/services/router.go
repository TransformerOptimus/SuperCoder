package services

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// ModelRoute maps a model prefix to a provider name.
type ModelRoute struct {
	Prefix       string
	ProviderName string
}

// Router performs config-driven model→provider routing.
type Router struct {
	mu              sync.RWMutex
	providers       map[string]ProviderAdapter
	modelPrefixes   []ModelRoute
	defaultProvider string
	logger          *zap.Logger
}

func NewRouter(logger *zap.Logger, defaultProvider string) *Router {
	return &Router{
		providers:       make(map[string]ProviderAdapter),
		defaultProvider: defaultProvider,
		logger:          logger.Named("gateway.router"),
	}
}

// RegisterProvider adds a provider adapter with its model prefix routes.
func (r *Router) RegisterProvider(name string, adapter ProviderAdapter, prefixes []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = adapter
	for _, p := range prefixes {
		r.modelPrefixes = append(r.modelPrefixes, ModelRoute{
			Prefix:       p,
			ProviderName: name,
		})
	}
	// Sort by prefix length descending for longest-prefix match
	sort.Slice(r.modelPrefixes, func(i, j int) bool {
		return len(r.modelPrefixes[i].Prefix) > len(r.modelPrefixes[j].Prefix)
	})
}

// Route selects a provider adapter for the given model.
// providerOverride (from X-Provider header) takes precedence, then longest-prefix match,
// then falls back to default provider.
func (r *Router) Route(model, providerOverride string) (ProviderAdapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if providerOverride != "" {
		if adapter, ok := r.providers[providerOverride]; ok {
			r.logger.Debug("routed by provider override",
				zap.String("model", model),
				zap.String("provider", providerOverride),
			)
			return adapter, nil
		}
		return nil, fmt.Errorf("unknown provider override: %s", providerOverride)
	}

	for _, route := range r.modelPrefixes {
		if strings.HasPrefix(model, route.Prefix) {
			if adapter, ok := r.providers[route.ProviderName]; ok {
				r.logger.Debug("routed by model prefix",
					zap.String("model", model),
					zap.String("prefix", route.Prefix),
					zap.String("provider", route.ProviderName),
				)
				return adapter, nil
			}
		}
	}

	if adapter, ok := r.providers[r.defaultProvider]; ok {
		r.logger.Debug("routed to default provider",
			zap.String("model", model),
			zap.String("provider", r.defaultProvider),
		)
		return adapter, nil
	}

	return nil, fmt.Errorf("no provider found for model: %s", model)
}
