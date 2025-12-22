package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sync"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// luaGCRA implements Generic Cell Rate Algorithm.
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

// emergencyLimiter handles traffic when Redis is down.
// It uses a global token bucket, which is coarser than per-user limits,
// but protects the database from total meltdown.
var (
	emergencyLimiter *rate.Limiter
	limiterOnce      sync.Once
)

// RateLimitMiddleware applies a static rate limit using Redis GCRA.
// Added Circuit Breaker pattern. If Redis fails, fall back to in-memory rate limiting.
// This prevents "Fail Open" from becoming "Database DDoS".
func RateLimitMiddleware(rdb *redis.Client, rps int, burst int, period time.Duration) func(http.Handler) http.Handler {
	// Initialize emergency limiter (Allow 2x normal traffic globally as fallback)
	limiterOnce.Do(func() {
		// Calculate global fallback rate (rough estimation)
		// Assuming we want to survive, we allow some burst but limit sustained load.
		emergencyLimiter = rate.NewLimiter(rate.Limit(rps*2), burst*2)
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fail-open if disabled (rate 0)
			if rps <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Resolve Identity
			var identity string
			if user := r.Context().Value(AuthPrincipalIDKey); user != nil {
				identity = fmt.Sprintf("user:%v", user)
			} else {
				identity = "ip:" + getRealIP(r)
			}

			key := fmt.Sprintf("rl:%s", identity)

			// Execute Redis GCRA
			res, err := luaGCRA.Run(r.Context(), rdb, []string{key}, rps, period.Seconds(), burst).Float64()

			if err != nil {
				// REDIS DOWN -> FALLBACK MODE
				// Instead of blindly failing open, we check local limiter.
				if !emergencyLimiter.Allow() {
					w.Header().Set("X-RateLimit-Fallback", "true")
					http.Error(w, "Service Unavailable (Rate Limit Fallback)", http.StatusServiceUnavailable)
					return
				}

				// Redis down, but local limit allows. Proceed with caution.
				next.ServeHTTP(w, r)
				return
			}

			// Handle Redis Limit Exceeded
			if res >= 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(res)))
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rps))
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rps))
			next.ServeHTTP(w, r)
		})
	}
}

// getRealIP attempts to extract the true client IP from headers.
func getRealIP(r *http.Request) string {
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		parts := strings.Split(xForwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}
	xRealIP := r.Header.Get("X-Real-Ip")
	if xRealIP != "" {
		return xRealIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
