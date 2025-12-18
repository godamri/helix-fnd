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
}

// IdempotencyMiddleware ensures that requests with the same Idempotency-Key
// are not processed concurrently. It does NOT cache responses.
//
// Strategy:
//  1. Client sends Idempotency-Key: <uuid>
//  2. We try to set this key in Redis with NX (Not Exists).
//  3. If SET succeeds -> Process request.
//  4. If SET fails -> Request is already in progress or was recently processed.
//     Return 409 Conflict (or 429).
//
// This protects against "double-click" submits and replay attacks.
// It DOES NOT protect against retrying a failed transaction days later;
// that belongs in the database constraints.
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

			// 1. Try to acquire lock
			// SetNX: key, "locked", expiry
			start := time.Now()
			success, err := cfg.RedisClient.SetNX(r.Context(), redisKey, "locked", cfg.Expiry).Result()

			if err != nil {
				cfg.Logger.Error("redis error in idempotency", "error", err)
				// Fail open or closed? Closed is safer for data integrity.
				http.Error(w, "Idempotency check failed", http.StatusInternalServerError)
				return
			}

			if !success {
				// Key exists. Request is either in progress or recently done.
				// We return 409 Conflict to tell the client "We saw this ID recently".
				// The client should check the status of the resource (GET) or generate a new ID if it's a new attempt.
				cfg.Logger.Warn("idempotency conflict", "key", key, "ip", r.RemoteAddr)
				w.Header().Set("Retry-After", "5") // Hint
				http.Error(w, "Duplicate request detected", http.StatusConflict)
				return
			}

			// Process Request
			// We do NOT defer delete here because we want the key to effectively "deduplicate"
			// requests for the duration of 'cfg.Expiry'.
			next.ServeHTTP(w, r)

			// Optional: Update key TTL or value based on success/failure if needed.
			// For now, simple "deduplication window" is sufficient and safest (OOM proof).
			_ = start
		})
	}
}
