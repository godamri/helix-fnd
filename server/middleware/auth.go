package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/godamri/helix-fnd/pkg/contextx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type AuthPayload struct {
	Headers    map[string]string
	RemoteAddr string
	Method     string
	Path       string
}

type AuthStrategy interface {
	Authenticate(ctx context.Context, payload AuthPayload) (context.Context, error)
}

type AuthMiddleware struct {
	strategy AuthStrategy
}

func NewAuthMiddleware(strategy AuthStrategy) *AuthMiddleware {
	return &AuthMiddleware{
		strategy: strategy,
	}
}

func (m *AuthMiddleware) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := contextx.WithEntryPoint(r.Context(), "http")

		headers := make(map[string]string)
		for k, v := range r.Header {
			if len(v) > 0 {
				headers[http.CanonicalHeaderKey(k)] = v[0]
			}
		}

		payload := AuthPayload{
			Headers:    headers,
			RemoteAddr: r.RemoteAddr,
			Method:     r.Method,
			Path:       r.URL.Path,
		}

		ctx, err := m.strategy.Authenticate(ctx, payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthMiddleware) GRPCUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ctx = contextx.WithEntryPoint(ctx, "grpc")

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "metadata is not provided")
	}

	headers := make(map[string]string)
	for k, v := range md {
		if len(v) > 0 {
			headers[http.CanonicalHeaderKey(k)] = v[0]
		}
	}

	remoteAddr := "0.0.0.0:0"
	if p, ok := peer.FromContext(ctx); ok {
		remoteAddr = p.Addr.String()
	}

	payload := AuthPayload{
		Headers:    headers,
		RemoteAddr: remoteAddr,
		Method:     info.FullMethod,
		Path:       info.FullMethod,
	}

	newCtx, err := m.strategy.Authenticate(ctx, payload)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	return handler(newCtx, req)
}

func (p *AuthPayload) GetHeader(key string) string {
	if v, ok := p.Headers[http.CanonicalHeaderKey(key)]; ok {
		return v
	}
	key = strings.ToLower(key)
	for k, v := range p.Headers {
		if strings.ToLower(k) == key {
			return v
		}
	}
	return ""
}
