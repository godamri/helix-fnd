package feature

import (
	"context"
	"os"
	"strings"
	"sync"
)

// Provider defines how we fetch flags.
type Provider interface {
	IsEnabled(ctx context.Context, key string) bool
}

// Manager is the main entry point.
type Manager struct {
	provider Provider
}

var (
	globalManager *Manager
	once          sync.Once
)

// Init initializes the global feature manager.
// Default to EnvProvider if provider is nil.
func Init(p Provider) {
	once.Do(func() {
		if p == nil {
			p = &EnvProvider{}
		}
		globalManager = &Manager{provider: p}
	})
}

// IsEnabled checks the feature flag.
func IsEnabled(ctx context.Context, key string) bool {
	if globalManager == nil {
		return false // Default safe
	}
	return globalManager.provider.IsEnabled(ctx, key)
}

// EnvProvider is a simple provider reading from Environment Variables.
// Format: FEATURE_MY_NEW_FEATURE=true
type EnvProvider struct{}

func (e *EnvProvider) IsEnabled(ctx context.Context, key string) bool {
	envKey := "FEATURE_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
	val := os.Getenv(envKey)
	return strings.ToLower(val) == "true" || val == "1"
}
