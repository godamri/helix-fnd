package app

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/kelseyhightower/envconfig"
)

// Loader standardizes how we load configuration.
// It combines envconfig (for parsing) and validator/v10 (for enforcement).
type Loader struct {
	validate *validator.Validate
}

func NewConfigLoader() *Loader {
	return &Loader{
		validate: validator.New(),
	}
}

// Load reads env vars into the provided spec struct and validates it.
// If this fails, the application SHOULD panic and die.
func (l *Loader) Load(ctx context.Context, spec interface{}, prefix string) error {
	// 1. Parse Env Vars
	if err := envconfig.Process(prefix, spec); err != nil {
		return fmt.Errorf("config: failed to process env vars: %w", err)
	}

	// 2. Validate Constraints (min, max, required, etc.)
	if err := l.validate.Struct(spec); err != nil {
		return fmt.Errorf("config: validation failed: %w", err)
	}

	return nil
}
