package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type IdempotencyMode string

const (
	ModeHeaderOnly IdempotencyMode = "HEADER_ONLY"
)

type IdempotencyConfig struct {
	Mode        IdempotencyMode
	HeaderKey   string
	Expiry      time.Duration
	RedisClient *redis.Client
	Logger      *slog.Logger
}

// StoredResponse is what we cache in Redis.
type StoredResponse struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

type responseCapturer struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (w *responseCapturer) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseCapturer) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func IdempotencyMiddleware(cfg IdempotencyConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip Safe Methods
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get(cfg.HeaderKey)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// We MUST use the authenticated user ID to prevent cross-tenant collisions.
			principalID, ok := r.Context().Value(AuthPrincipalIDKey).(string)
			if !ok || principalID == "" {
				// If endpoint is public/unauthenticated, we fallback to IP, but prefix strictly.
				// Ideally, critical transactional endpoints should ALWAYS be authenticated.
				principalID = "anon_ip:" + r.RemoteAddr
			}

			// Format: idempotency:{user_id}:{client_key}
			redisKey := fmt.Sprintf("idempotency:%s:%s", principalID, key)
			ctx := r.Context()

			// Check State (SETNX for locking)
			processingTTL := 30 * time.Second
			acquired, err := cfg.RedisClient.SetNX(ctx, redisKey, "PROCESSING", processingTTL).Result()
			if err != nil {
				cfg.Logger.Error("idempotency: redis error", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if !acquired {
				val, err := cfg.RedisClient.Get(ctx, redisKey).Result()
				if err != nil {
					next.ServeHTTP(w, r)
					return
				}

				if val == "PROCESSING" {
					http.Error(w, "Request is currently being processed", http.StatusConflict)
					return
				}

				var stored StoredResponse
				if jsonErr := json.Unmarshal([]byte(val), &stored); jsonErr == nil {
					cfg.Logger.Info("Idempotency Hit", "key", key, "user", principalID)
					for k, v := range stored.Headers {
						for _, val := range v {
							w.Header().Add(k, val)
						}
					}
					w.Header().Set("X-Idempotency-Hit", "true")
					w.WriteHeader(stored.Status)
					w.Write(stored.Body)
					return
				}

				// Corrupted cache -> Fail open (reprocess)
				cfg.Logger.Warn("Idempotency cache corrupted, reprocessing", "key", redisKey)
			}

			capturer := &responseCapturer{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(capturer, r)

			if capturer.statusCode == 0 || capturer.statusCode >= 500 {
				// Don't cache internal server errors or crashes. Retry should be allowed.
				cfg.RedisClient.Del(ctx, redisKey)
				return
			}

			respToStore := StoredResponse{
				Status:  capturer.statusCode,
				Headers: capturer.Header(),
				Body:    capturer.body.Bytes(),
			}

			data, _ := json.Marshal(respToStore)
			cfg.RedisClient.Set(ctx, redisKey, data, cfg.Expiry)
		})
	}
}
