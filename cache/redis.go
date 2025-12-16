package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Addr     string `envconfig:"REDIS_ADDR" required:"true"`
	Password string `envconfig:"REDIS_PASSWORD" default:""`
	DB       int    `envconfig:"REDIS_DB" default:"0"`
}

// NewRedis initializes a Redis client and performs a fail-fast ping.
// If Redis is unreachable at startup, the app dies. This is a feature, not a bug.
func NewRedis(ctx context.Context, cfg Config) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Fail fast: Verify connection immediately.
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("cache: failed to connect to redis at %s: %w", cfg.Addr, err)
	}

	return rdb, nil
}
