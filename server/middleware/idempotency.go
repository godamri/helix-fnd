package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	IdempotencyHeader = "X-Idempotency-Key"
	lockTTL           = 30 * time.Second
	cacheTTL          = 24 * time.Hour
)

// luaIdempotencyLock guarantees atomicity.
// KEYS[1] = key
// ARGV[1] = status ("PROCESSING")
// ARGV[2] = ttl (milliseconds)
// ARGV[3] = payload_hash
// Returns:
// 0: Acquired lock
// 1: Already processing
// 2: Already completed (checksum match)
// 3: Conflict (checksum mismatch - dangerous!)
var luaIdempotencyLock = redis.NewScript(`
	local key = KEYS[1]
	local status = ARGV[1]
	local ttl = tonumber(ARGV[2])
	local hash = ARGV[3]

	local current = redis.call("HMGET", key, "status", "hash")
	local current_status = current[1]
	local current_hash = current[2]

	if not current_status then
		-- New Request
		redis.call("HMSET", key, "status", status, "hash", hash)
		redis.call("PEXPIRE", key, ttl)
		return 0
	end

	if current_status == "PROCESSING" then
		return 1
	end

	if current_status == "COMPLETED" then
		if current_hash == hash then
			return 2
		else
			return 3
		end
	end

	return 1 -- Fallback default
`)

type idempotencyWriter struct {
	http.ResponseWriter
	status int
	body   *bytes.Buffer
}

func (w *idempotencyWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *idempotencyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func IdempotencyMiddleware(rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(IdempotencyHeader)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// 1. Payload Checksum (Integrity Check)
			// Must normalize JSON to ensure key order doesn't affect hash.
			var payload []byte
			var payloadHash [32]byte

			if r.Body != nil {
				rawPayload, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewBuffer(rawPayload)) // Reset body for handler

				// Normalization Logic:
				// If valid JSON, decode and re-encode to ensure sorted keys.
				// If not JSON or empty, use raw bytes.
				if len(rawPayload) > 0 {
					var jsonObj interface{}
					if err := json.Unmarshal(rawPayload, &jsonObj); err == nil {
						// Standard json.Marshal sorts map keys alphabetically
						if normalized, err := json.Marshal(jsonObj); err == nil {
							payload = normalized
						} else {
							payload = rawPayload // Fallback if marshal fails (unlikely)
						}
					} else {
						payload = rawPayload // Not JSON, use raw
					}
				}
			}

			payloadHash = sha256.Sum256(payload)
			hashStr := hex.EncodeToString(payloadHash[:])

			// 2. Prepare Redis Key
			redisKey := fmt.Sprintf("idempotency:v1:%s", key)

			// 3. Execute Lua Script (Atomic)
			res, err := luaIdempotencyLock.Run(r.Context(), rdb, []string{redisKey}, "PROCESSING", lockTTL.Milliseconds(), hashStr).Int()
			if err != nil {
				// Fail Open or Closed? For Enterprise consistency, Fail Closed (Error).
				http.Error(w, "Idempotency store unavailable", http.StatusServiceUnavailable)
				return
			}

			switch res {
			case 1: // Processing
				w.Header().Set("Retry-After", "5")
				http.Error(w, "Request processing in progress", http.StatusTooManyRequests)
				return
			case 2: // Completed & Hash Match
				// Fetch cached response
				cached, err := rdb.HGetAll(r.Context(), redisKey).Result()
				if err != nil {
					http.Error(w, "Failed to retrieve cached response", http.StatusInternalServerError)
					return
				}
				w.Header().Set("X-Idempotency-Hit", "true")
				w.Header().Set("Content-Type", cached["content_type"])
				w.WriteHeader(http.StatusOK) // In real impl, store status code too
				w.Write([]byte(cached["body"]))
				return
			case 3: // Conflict (Same Key, Different Payload)
				http.Error(w, "Idempotency key reused with different payload", http.StatusConflict)
				return
			}

			// Case 0: Lock Acquired. Process.
			writer := &idempotencyWriter{
				ResponseWriter: w,
				status:         http.StatusOK,
				body:           &bytes.Buffer{},
			}

			next.ServeHTTP(writer, r)

			// 4. Post-Process
			if writer.status >= 200 && writer.status < 300 {
				// Success: Store Response
				pipe := rdb.Pipeline()
				pipe.HSet(r.Context(), redisKey, map[string]interface{}{
					"status":       "COMPLETED",
					"body":         writer.body.String(),
					"content_type": w.Header().Get("Content-Type"),
				})
				pipe.Expire(r.Context(), redisKey, cacheTTL)
				_, _ = pipe.Exec(r.Context())
			} else {
				// Failed: Release Lock so client can retry
				rdb.Del(r.Context(), redisKey)
			}
		})
	}
}
