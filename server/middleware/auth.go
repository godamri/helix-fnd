package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/godamri/helix-fnd/crypto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthPrincipalIDKey and AuthPrincipalTypeKey should be defined in a shared context package
// but for Foundation integrity we define context keys here if not present.
type contextKey string

const (
	AuthPrincipalIDKey   contextKey = "helix_auth_principal_id"
	AuthPrincipalTypeKey contextKey = "helix_auth_principal_type"
)

type AuthMiddlewareFactory struct {
	verifier crypto.JWKSVerifier
}

func NewAuthMiddlewareFactory(verifier crypto.JWKSVerifier) *AuthMiddlewareFactory {
	if verifier == nil {
		panic("AuthMiddlewareFactory requires a non-nil JWKSVerifier")
	}
	return &AuthMiddlewareFactory{verifier: verifier}
}

// HTTPMiddleware enforces Bearer token verification on HTTP routes.
func (f *AuthMiddlewareFactory) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// If optional auth is needed, logic changes. Here we enforce it.
			// Or pass through and let handlers check context.
			// Assuming strict enforcement for applied routes.
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
			return
		}

		claims, err := f.verifier.VerifyToken(parts[1])
		if err != nil {
			http.Error(w, "Invalid token: "+err.Error(), http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, AuthPrincipalIDKey, claims.UserID)
		ctx = context.WithValue(ctx, AuthPrincipalTypeKey, claims.Type)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GRPCUnaryInterceptor enforces Bearer token verification on gRPC calls.
func (f *AuthMiddlewareFactory) GRPCUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return handler(ctx, req)
	}

	values := md["authorization"]
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	authHeader := values[0]
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
	}

	claims, err := f.verifier.VerifyToken(parts[1])
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	ctx = context.WithValue(ctx, AuthPrincipalIDKey, claims.UserID)
	ctx = context.WithValue(ctx, AuthPrincipalTypeKey, claims.Type)

	return handler(ctx, req)
}
