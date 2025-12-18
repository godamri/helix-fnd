package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type IdempotencyMode string

const (
	ModeHeaderOnly IdempotencyMode = "HEADER_ONLY" // Key: idempotency:{token}
	ModeBodyHash   IdempotencyMode = "BODY_HASH"   // Key: idempotency:{token}:{body_hash}
)

type IdempotencyConfig struct {
	Mode        IdempotencyMode
	HeaderKey   string
	Expiry      time.Duration
	RedisClient *redis.Client
	Logger      *slog.Logger
}

func IdempotencyMiddleware(cfg IdempotencyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if safe method (GET, HEAD, OPTIONS)
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Get Idempotency Key
			key := r.Header.Get(cfg.HeaderKey)
			if key == "" {
				// If required, reject. For now, we allow pass-through if missing (optional idempotency).
				// Strict mode can be added later.
				next.ServeHTTP(w, r)
				return
			}

			// Determine Redis Key based on Mode
			var redisKey string
			switch cfg.Mode {
			case ModeBodyHash:
				hash, err := generateBodyHash(r)
				if err != nil {
					cfg.Logger.Error("idempotency: failed to hash body", "error", err)
					http.Error(w, "Invalid Request Body", http.StatusBadRequest)
					return
				}
				redisKey = fmt.Sprintf("idempotency:%s:%s", key, hash)
			default:
				redisKey = fmt.Sprintf("idempotency:%s", key)
			}

			// Check Redis (SETNX - Set if Not Exists)
			// We store "PROCESSING" status first to handle concurrent race conditions.
			acquired, err := cfg.RedisClient.SetNX(r.Context(), redisKey, "PROCESSING", cfg.Expiry).Result()
			if err != nil {
				cfg.Logger.Error("idempotency: redis error", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if !acquired {
				// Key exists. Check if it's still processing or done.
				val, _ := cfg.RedisClient.Get(r.Context(), redisKey).Result()
				if val == "PROCESSING" {
					http.Error(w, "Request already in progress", http.StatusConflict)
					return
				}
				// If we had stored the response, we could return it here.
				// For now, simple idempotency rejection:
				http.Error(w, "Request already processed", http.StatusConflict)
				return
			}

			// Execute Handler
			// Capture response writer to store result later if needed (Phase 3 enhancement).
			// For now, we just proceed.
			next.ServeHTTP(w, r)

			// Update status (Optional: Store response)
			// In a full implementation, you'd wrap ResponseWriter, capture status/body, and store it in Redis.
			// Here we keep the lock 'true' to prevent replays.
			cfg.RedisClient.Set(r.Context(), redisKey, "COMPLETED", cfg.Expiry)
		})
	}
}

// generateBodyHash creates a SHA256 hash of the canonicalized JSON body.
// It reads r.Body and restores it for the next handler.
func generateBodyHash(r *http.Request) (string, error) {
	if r.Body == nil {
		return "", nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	// Restore body
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if len(bodyBytes) == 0 {
		return "empty", nil
	}

	// Canonicalization: Unmarshal to interface{} and Marshal back.
	// encoding/json sorts map keys, providing basic canonicalization.
	var body interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return "", err // Not valid JSON, maybe return raw hash or error
	}

	canonicalBytes, _ := json.Marshal(body)
	hash := sha256.Sum256(canonicalBytes)
	return hex.EncodeToString(hash[:]), nil
}
