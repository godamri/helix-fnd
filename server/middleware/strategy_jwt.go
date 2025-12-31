package middleware

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/godamri/helix-fnd/crypto"
	"github.com/godamri/helix-fnd/pkg/contextx"
)

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
	authHeader := payload.GetHeader("Authorization")
	if authHeader == "" {
		return nil, errors.New("missing authorization header")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return nil, errors.New("invalid authorization header format")
	}

	tokenStr := parts[1]

	claims, err := s.verifier.VerifyToken(tokenStr)
	if err != nil {
		s.logger.WarnContext(ctx, "JWT verification failed", "error", err, "ip", payload.RemoteAddr)
		return nil, errors.New("invalid token")
	}

	ctx = contextx.WithAuthMethod(ctx, "jwt")

	if claims.ID != "" {
		ctx = contextx.WithSessionID(ctx, claims.ID)
	}

	ctx = contextx.WithIdentity(
		ctx,
		claims.Subject,        // UserID
		claims.OrgID,          // OrgID
		claims.Email,          // Email
		claims.GetActorType(), // human | service | system
		claims.GetRoles(),     // Roles/Permissions
	)

	return ctx, nil
}
