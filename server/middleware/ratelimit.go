package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// luaGCRA implements Generic Cell Rate Algorithm.
// This provides smooth traffic shaping unlike Fixed Window.
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

// RateLimitMiddleware applies a static rate limit using Redis GCRA.
// Configuration is fixed at startup. To change limits, restart the service (Immutable Infrastructure).
func RateLimitMiddleware(rdb *redis.Client, rate int, burst int, period time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fail-open if disabled (rate 0)
			if rate <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Resolve Identity
			// Prioritize Authenticated User > Real IP
			var identity string
			if user := r.Context().Value(AuthPrincipalIDKey); user != nil {
				identity = fmt.Sprintf("user:%v", user)
			} else {
				identity = "ip:" + getRealIP(r)
			}

			key := fmt.Sprintf("rl:%s", identity)

			// Execute GCRA
			res, err := luaGCRA.Run(r.Context(), rdb, []string{key}, rate, period.Seconds(), burst).Float64()
			if err != nil {
				// Redis failure -> Fail Open to preserve availability
				next.ServeHTTP(w, r)
				return
			}

			// Handle Limit Exceeded
			if res >= 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(res)))
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rate))
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rate))
			next.ServeHTTP(w, r)
		})
	}
}

// getRealIP attempts to extract the true client IP from headers.
// TRUST BOUNDARY: Only trust X-Forwarded-For if you are behind a trusted Ingress/LB.
func getRealIP(r *http.Request) string {
	// Standard Proxy Header
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		// X-Forwarded-For: client, proxy1, proxy2
		// We take the first one (Client IP).
		parts := strings.Split(xForwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}

	// Nginx often set this single header
	xRealIP := r.Header.Get("X-Real-Ip")
	if xRealIP != "" {
		return xRealIP
	}

	// Fallback to direct connection
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
