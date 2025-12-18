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
			// IP Resolution Risk
			// Previously: r.RemoteAddr (Internal K8s IP).
			// Now: Prioritize UserID -> X-Forwarded-For -> RemoteAddr.

			var identity string

			// 1. Identity based on Auth (Most Secure/Accurate)
			if user := r.Context().Value(AuthPrincipalIDKey); user != nil {
				identity = fmt.Sprintf("user:%v", user)
			} else {
				// Fallback to IP
				// WARNING: This assumes your Load Balancer/Ingress strips untrusted X-Forwarded-For headers
				// and appends the real one. If exposed directly to internet, this is spoofable.
				clientIP := getRealIP(r)
				identity = "ip:" + clientIP
			}

			key := fmt.Sprintf("rl:%s", identity)

			// Execute GCRA
			res, err := luaGCRA.Run(r.Context(), rdb, []string{key}, rate, period.Seconds(), burst).Float64()
			if err != nil {
				// Fail Open: Redis down? Let traffic pass.
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

// getRealIP attempts to extract the true client IP from headers.
// It supports standard X-Forwarded-For and X-Real-Ip.
func getRealIP(r *http.Request) string {
	// Standard Proxy Header
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		// X-Forwarded-For: client, proxy1, proxy2
		// We take the first one (Client IP).
		// NOTE: In a trusted internal network/Ingress, this is safe.
		parts := strings.Split(xForwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}

	// Nginx/Envoy often set this single header
	xRealIP := r.Header.Get("X-Real-Ip")
	if xRealIP != "" {
		return xRealIP
	}

	// Fallback to direct connection (Localhost / Dev / No Proxy)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
