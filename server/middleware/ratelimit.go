package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// luaGCRA implements Generic Cell Rate Algorithm.
// A sophisticated leaky bucket variation.
// KEYS[1] = rate_limit_key
// ARGV[1] = rate (requests per period)
// ARGV[2] = period (seconds)
// ARGV[3] = burst (max concurrent)
// Returns:
// >= 0: Limited. Value is retry_after (seconds).
// -1: Allowed.
var luaGCRA = redis.NewScript(`
	local key = KEYS[1]
	local rate = tonumber(ARGV[1])
	local period = tonumber(ARGV[2])
	local burst = tonumber(ARGV[3])
	
	local emission_interval = period / rate
	local now = redis.call("TIME")
	local now_sec = tonumber(now[1])
	local now_usec = tonumber(now[2])
	local now_ts = now_sec + (now_usec / 1000000)

	local tat = redis.call("GET", key)
	
	if not tat then
		tat = now_ts
	else
		tat = tonumber(tat)
	end

	tat = math.max(now_ts, tat)
	
	local new_tat = tat + emission_interval
	local allow_at = new_tat - (burst * emission_interval)

	if allow_at <= now_ts then
		redis.call("SET", key, new_tat, "EX", math.ceil(period * 2))
		return -1
	end

	return math.ceil(allow_at - now_ts)
`)

// RateLimitMiddleware uses GCRA to provide smooth, burst-tolerant rate limiting.
func RateLimitMiddleware(rdb *redis.Client, rate int, period time.Duration, burst int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Identification Strategy:
			// 1. User ID (if Authenticated)
			// 2. IP Address (Fallback)
			identity := r.RemoteAddr
			if user := r.Context().Value("user_id"); user != nil {
				identity = fmt.Sprintf("user:%v", user)
			}

			key := fmt.Sprintf("rl:%s", identity)

			// Execute GCRA
			res, err := luaGCRA.Run(r.Context(), rdb, []string{key}, rate, period.Seconds(), burst).Float64()
			if err != nil {
				// Fail Open: If Redis is down, don't block users. Log it!
				// slog.Error("Rate limit error", "err", err)
				next.ServeHTTP(w, r)
				return
			}

			if res >= 0 {
				retryAfter := strconv.Itoa(int(res))
				w.Header().Set("Retry-After", retryAfter)
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rate))
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rate))
			next.ServeHTTP(w, r)
		})
	}
}
