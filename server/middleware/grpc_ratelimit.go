package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// GRPCRateLimitInterceptor applies GCRA rate limiting to gRPC unary calls.
// It prioritizes AuthPrincipalIDKey if present (authenticated service/user),
// otherwise falls back to the peer's remote IP address.
func GRPCRateLimitInterceptor(rdb *redis.Client, rate int, burst int, period time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Rule: Boring beats clever. If rate is 0 or negative, skip the check.
		if rate <= 0 {
			return handler(ctx, req)
		}

		// Rule: Explicit identity identification.
		var identity string
		if user := ctx.Value(AuthPrincipalIDKey); user != nil {
			identity = fmt.Sprintf("user:%v", user)
		} else {
			identity = "ip:unknown"
			if p, ok := peer.FromContext(ctx); ok {
				identity = "ip:" + p.Addr.String()
			}
		}

		key := fmt.Sprintf("rl:grpc:%s", identity)

		// Rule: Survivability. Fail open if Redis is unreachable.
		// We use the existing luaGCRA script from the HTTP middleware.
		res, err := luaGCRA.Run(ctx, rdb, []string{key}, rate, period.Seconds(), burst).Float64()
		if err != nil {
			// Do not block the launch just because the cache is down.
			return handler(ctx, req)
		}

		// Rule: Predictable behavior. If limit exceeded, return standard gRPC code.
		if res >= 0 {
			retryAfter := strconv.Itoa(int(res))

			// Inject metadata so client-side interceptors can handle backoff.
			header := metadata.Pairs(
				"x-retry-after", retryAfter,
				"x-ratelimit-limit", strconv.Itoa(rate),
			)
			_ = grpc.SetHeader(ctx, header)

			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded, retry in %s seconds", retryAfter)
		}

		return handler(ctx, req)
	}
}
