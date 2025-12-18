package config

import (
	"fmt"
	"os"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v3"
)

// Loader handles configuration loading from YAML and Environment variables.
// Priority: Env Vars > YAML > Defaults.
// This loader is immutable. It runs once at startup.
type Loader[T any] struct {
	envPrefix  string
	configPath string
}

func NewLoader[T any](envPrefix, configPath string) *Loader[T] {
	return &Loader[T]{
		envPrefix:  envPrefix,
		configPath: configPath,
	}
}

// Load reads the configuration.
func (l *Loader[T]) Load() (*T, error) {
	var cfg T

	// 1. Load from YAML if exists
	if l.configPath != "" {
		if _, err := os.Stat(l.configPath); err == nil {
			file, err := os.Open(l.configPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open config file: %w", err)
			}
			defer file.Close()

			decoder := yaml.NewDecoder(file)
			if err := decoder.Decode(&cfg); err != nil {
				return nil, fmt.Errorf("failed to decode config file: %w", err)
			}
		}
	}

	// 2. Override with Environment Variables
	if err := envconfig.Process(l.envPrefix, &cfg); err != nil {
		return nil, fmt.Errorf("failed to process env vars: %w", err)
	}

	return &cfg, nil
}
