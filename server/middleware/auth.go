package middleware

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// --- Context Keys (The Single Source of Truth) ---
type contextKey string

const (
	AuthPrincipalIDKey    contextKey = "helix_auth_principal_id"    // string (UUID)
	AuthPrincipalTypeKey  contextKey = "helix_auth_principal_type"  // string ("user", "service")
	AuthPrincipalRoleKey  contextKey = "helix_auth_principal_roles" // []string
	AuthPrincipalEmailKey contextKey = "helix_auth_principal_email" // string
)

// --- The Strategy Interface (Inversion of Control) ---
type AuthStrategy interface {
	Authenticate(r *http.Request) (context.Context, error)
}

// --- The Factory (Agnostic Middleware Generator) ---
type AuthMiddleware struct {
	strategy AuthStrategy
}

func NewAuthMiddleware(strategy AuthStrategy) *AuthMiddleware {
	return &AuthMiddleware{
		strategy: strategy,
	}
}

// HTTPMiddleware: Standard interceptor
func (m *AuthMiddleware) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := m.strategy.Authenticate(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GRPCUnaryInterceptor: Standard interceptor
func (m *AuthMiddleware) GRPCUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Extract Metadata sebagai Headers
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "metadata is not provided")
	}

	// Construct HTTP Request
	mockReq := &http.Request{
		Header: make(http.Header),
		URL:    &url.URL{},
	}

	// Map Metadata to Header (Normalize keys)
	for k, v := range md {
		for _, val := range v {
			mockReq.Header.Add(k, val)
		}
	}

	// Extract Peer IP (Untuk TrustedHeaderStrategy CIDR Check)
	if p, ok := peer.FromContext(ctx); ok {
		mockReq.RemoteAddr = p.Addr.String()
	} else {
		mockReq.RemoteAddr = "0.0.0.0:0" // Fallback
	}

	// Delegate to Strategy (Reusing Logic)
	newCtx, err := m.strategy.Authenticate(mockReq)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	return handler(newCtx, req)
}

// Helper untuk gRPC Metadata extraction (Internal use - Legacy Support)
func grpcExtractToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "metadata is not provided")
	}
	values := md["authorization"]
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "authorization token is not provided")
	}
	authHeader := values[0]
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", status.Error(codes.Unauthenticated, "invalid auth header format")
	}
	return parts[1], nil
}
