package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

type TrustedHeaderStrategy struct {
	trustedCIDRs []*net.IPNet
	logger       *slog.Logger

	// Header Config
	headerUserID string
	headerRoles  string
	headerEmail  string
}

// Config untuk Strategy ini
type TrustedHeaderConfig struct {
	TrustedProxies []string // e.g. ["127.0.0.1/32", "10.0.0.0/8"]
	HeaderUserID   string   // default: X-Helix-User-ID
	HeaderRoles    string   // default: X-Helix-Role (Comma separated)
	HeaderEmail    string   // default: X-Helix-Email
}

func NewTrustedHeaderStrategy(cfg TrustedHeaderConfig, logger *slog.Logger) (*TrustedHeaderStrategy, error) {
	if len(cfg.TrustedProxies) == 0 {
		return nil, errors.New("security_risk: trusted_proxies list cannot be empty in gateway mode")
	}

	cidrs := make([]*net.IPNet, 0, len(cfg.TrustedProxies))
	for _, cidr := range cfg.TrustedProxies {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Fallback: Try parsing single IP as /32
			if ip := net.ParseIP(cidr); ip != nil {
				_, ipNet, _ = net.ParseCIDR(cidr + "/32")
			} else {
				return nil, fmt.Errorf("invalid cidr configuration: %s", cidr)
			}
		}
		cidrs = append(cidrs, ipNet)
	}

	// Defaults
	if cfg.HeaderUserID == "" {
		cfg.HeaderUserID = "X-Helix-User-ID"
	}
	if cfg.HeaderRoles == "" {
		cfg.HeaderRoles = "X-Helix-Role"
	}
	if cfg.HeaderEmail == "" {
		cfg.HeaderEmail = "X-Helix-Email"
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &TrustedHeaderStrategy{
		trustedCIDRs: cidrs,
		logger:       logger,
		headerUserID: cfg.HeaderUserID,
		headerRoles:  cfg.HeaderRoles,
		headerEmail:  cfg.HeaderEmail,
	}, nil
}

func (s *TrustedHeaderStrategy) Authenticate(r *http.Request) (context.Context, error) {
	// SECURITY: IP Validation (The Gatekeeper)
	// Raw RemoteAddr (e.g., "10.1.2.3:45678")
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		s.logger.Warn("Auth rejected: failed to parse remote addr", "addr", r.RemoteAddr)
		return nil, errors.New("unauthorized gateway connection")
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, errors.New("invalid remote ip")
	}

	isTrusted := false
	for _, cidr := range s.trustedCIDRs {
		if cidr.Contains(ip) {
			isTrusted = true
			break
		}
	}

	if !isTrusted {
		// CRITICAL AUDIT LOG
		s.logger.WarnContext(r.Context(), "SECURITY ALERT: Untrusted IP attempted to spoof Gateway",
			"ip", host,
			"path", r.URL.Path,
		)
		return nil, errors.New("forbidden: untrusted source")
	}

	// DATA INTEGRITY: Header Extraction
	userID := r.Header.Get(s.headerUserID)
	if userID == "" {
		return nil, errors.New("missing identity header")
	}

	email := r.Header.Get(s.headerEmail)

	var roles []string
	rawRoles := r.Header.Get(s.headerRoles)
	if rawRoles != "" {
		split := strings.Split(rawRoles, ",")
		for _, role := range split {
			trimmed := strings.TrimSpace(role)
			if trimmed != "" {
				roles = append(roles, trimmed)
			}
		}
	} else {
		roles = []string{} // Ensure non-nil for safety
	}

	// Hydrate Context
	ctx := r.Context()
	ctx = context.WithValue(ctx, AuthPrincipalIDKey, userID)
	ctx = context.WithValue(ctx, AuthPrincipalTypeKey, "user") // Default assumption
	ctx = context.WithValue(ctx, AuthPrincipalRoleKey, roles)
	ctx = context.WithValue(ctx, AuthPrincipalEmailKey, email)

	return ctx, nil
}
