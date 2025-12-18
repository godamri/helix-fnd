package middleware

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/godamri/helix-fnd/crypto"
)

// JWTStrategy for Standalone/Dev mode.
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

func (s *JWTStrategy) Authenticate(ctx context.Context, payload AuthPayload) (context.Context, error) {
	// Extract Bearer Token
	authHeader := payload.GetHeader("Authorization")
	if authHeader == "" {
		return nil, errors.New("missing authorization header")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, errors.New("invalid authorization header format")
	}

	tokenStr := parts[1]

	// Verify
	claims, err := s.verifier.VerifyToken(tokenStr)
	if err != nil {
		s.logger.WarnContext(ctx, "JWT verification failed", "error", err, "ip", payload.RemoteAddr)
		return nil, errors.New("invalid token")
	}

	// Hydrate
	ctx = context.WithValue(ctx, AuthPrincipalIDKey, claims.Subject)
	ctx = context.WithValue(ctx, AuthPrincipalTypeKey, "user")
	ctx = context.WithValue(ctx, AuthPrincipalRoleKey, claims.GetRoles())
	ctx = context.WithValue(ctx, AuthPrincipalEmailKey, claims.Email)

	return ctx, nil
}
