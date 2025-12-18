package middleware

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type contextKey string

const (
	AuthPrincipalIDKey    contextKey = "helix_auth_principal_id"
	AuthPrincipalTypeKey  contextKey = "helix_auth_principal_type"
	AuthPrincipalRoleKey  contextKey = "helix_auth_principal_roles"
	AuthPrincipalEmailKey contextKey = "helix_auth_principal_email"
)

// AuthPayload decouples the strategy from the transport (HTTP/gRPC).
type AuthPayload struct {
	Headers    map[string]string
	RemoteAddr string
	Method     string
	Path       string
}

// AuthStrategy Interface (Protocol Agnostic)
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

// HTTPMiddleware: Adapts HTTP request to AuthPayload
func (m *AuthMiddleware) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		ctx, err := m.strategy.Authenticate(r.Context(), payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GRPCUnaryInterceptor: Adapts gRPC metadata to AuthPayload
func (m *AuthMiddleware) GRPCUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "metadata is not provided")
	}

	headers := make(map[string]string)
	for k, v := range md {
		if len(v) > 0 {
			// gRPC metadata keys are always lowercase
			headers[http.CanonicalHeaderKey(k)] = v[0]
			// Also keep original for flexibility if needed, but Canonical is safer for matching
			headers[k] = v[0]
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

// Helper to get Header value case-insensitively from the map
func (p *AuthPayload) GetHeader(key string) string {
	// Fast path
	if v, ok := p.Headers[key]; ok {
		return v
	}
	if v, ok := p.Headers[http.CanonicalHeaderKey(key)]; ok {
		return v
	}
	// Slow path (iterate) - rare if canonicalized correctly
	key = strings.ToLower(key)
	for k, v := range p.Headers {
		if strings.ToLower(k) == key {
			return v
		}
	}
	return ""
}
