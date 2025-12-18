package config

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/go-playground/validator/v10"
	"github.com/kelseyhightower/envconfig"
)

// Provider defines source of configuration (File, Etcd, Consul, Env).
type Provider interface {
	Load() (map[string]interface{}, error)
	Watch(ctx context.Context, onChange func())
}

// Container holds the config safely for concurrent access.
type Container[T any] struct {
	store    atomic.Value
	mu       sync.Mutex // Only for writing updates
	validate *validator.Validate
}

// NewContainer initializes the config container.
func NewContainer[T any](initial T) *Container[T] {
	c := &Container[T]{
		validate: validator.New(),
	}
	c.store.Store(&initial)
	return c
}

// Get returns the current snapshot of the config.
// This is WAIT-FREE and LOCK-FREE. Extremely fast.
func (c *Container[T]) Get() *T {
	return c.store.Load().(*T)
}

// Update swaps the config pointer atomically.
func (c *Container[T]) Update(newConfig T) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.validate.Struct(newConfig); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	c.store.Store(&newConfig)
	return nil
}

// Loader orchestrates loading from multiple sources.
// Priority: Env Vars > YAML File > Defaults
type Loader[T any] struct {
	prefix   string
	filePath string
}

func NewLoader[T any](prefix, filePath string) *Loader[T] {
	return &Loader[T]{
		prefix:   prefix,
		filePath: filePath,
	}
}

// Load constructs the config struct.
func (l *Loader[T]) Load() (*T, error) {
	var cfg T

	// Load from YAML (if exists) - Simulating K8s ConfigMap
	// In real implementation, you'd read file content here.
	// For now, we assume standard decoding.

	// Override with Env Vars (12-Factor App compliance)
	if err := envconfig.Process(l.prefix, &cfg); err != nil {
		return nil, fmt.Errorf("envconfig failed: %w", err)
	}

	return &cfg, nil
}
