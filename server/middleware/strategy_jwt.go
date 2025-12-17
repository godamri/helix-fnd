package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/godamri/helix-fnd/crypto"
)

// JWTStrategy untuk Standalone Mode (Dev/Legacy).
type JWTStrategy struct {
	verifier crypto.JWKSVerifier
	logger   *slog.Logger
}

func NewJWTStrategy(verifier crypto.JWKSVerifier, logger *slog.Logger) *JWTStrategy {
	if logger == nil {
		logger = slog.Default()
	}
	return &JWTStrategy{
		verifier: verifier,
		logger:   logger,
	}
}

func (s *JWTStrategy) Authenticate(r *http.Request) (context.Context, error) {
	// 1. Extract Bearer Token
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("missing authorization header")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, errors.New("invalid authorization header format")
	}

	tokenStr := parts[1]

	// Verify with JWKS
	claims, err := s.verifier.VerifyToken(tokenStr)
	if err != nil {
		s.logger.WarnContext(r.Context(), "JWT verification failed", "error", err, "ip", r.RemoteAddr)
		return nil, errors.New("invalid token")
	}

	// Hydrate Context (Data Parity Guaranteed by HelixClaims struct)
	ctx := r.Context()
	ctx = context.WithValue(ctx, AuthPrincipalIDKey, claims.Subject)
	ctx = context.WithValue(ctx, AuthPrincipalTypeKey, "user")
	ctx = context.WithValue(ctx, AuthPrincipalRoleKey, claims.GetRoles())
	ctx = context.WithValue(ctx, AuthPrincipalEmailKey, claims.Email)

	return ctx, nil
}
