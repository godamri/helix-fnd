package middleware

import (
	"net/http"
	"time"

	"log/slog"

	"github.com/redis/go-redis/v9"
)

type IdempotencyConfig struct {
	HeaderKey   string
	Expiry      time.Duration
	RedisClient *redis.Client
	Logger      *slog.Logger

	// True  = Availability First
	// False = Consistency First
	FailOpen bool
}

// IdempotencyMiddleware ensures that requests with the same Idempotency-Key
// are not processed concurrently.
//
// Strategy:
//
//	Client sends Idempotency-Key: <uuid>
//	We try to set this key in Redis with NX (Not Exists).
//	If SET succeeds -> Process request.
//	If SET fails -> Request is already in progress or was recently processed.
//	   Return 409 Conflict.
func IdempotencyMiddleware(cfg IdempotencyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(cfg.HeaderKey)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Prefix to avoid collision with other keys
			redisKey := "idempotency:" + key

			// Try to acquire lock
			// SetNX: key, "locked", expiry
			start := time.Now()
			success, err := cfg.RedisClient.SetNX(r.Context(), redisKey, "locked", cfg.Expiry).Result()

			if err != nil {
				// REDIS DOWN
				cfg.Logger.Error("idempotency: redis unreachable",
					"error", err,
					"key", key,
					"strategy", map[bool]string{true: "fail_open", false: "fail_closed"}[cfg.FailOpen],
				)

				if cfg.FailOpen {
					cfg.Logger.Warn("idempotency: bypassed check due to redis failure", "key", key)
					next.ServeHTTP(w, r)
					return
				}

				http.Error(w, "Idempotency check service unavailable", http.StatusServiceUnavailable)
				return
			}

			if !success {
				cfg.Logger.Warn("idempotency: conflict detected", "key", key, "ip", r.RemoteAddr)
				w.Header().Set("Retry-After", "5") // Hint client to wait
				http.Error(w, "Duplicate request detected", http.StatusConflict)
				return
			}

			next.ServeHTTP(w, r)

			_ = start
		})
	}
}
