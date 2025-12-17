package crypto

import (
	"github.com/golang-jwt/jwt/v5"
)

// HelixClaims represents the standard JWT claims used across the Helix Ecosystem (Citadel v2).
// This structure is now Agnostic (removed Keycloak-specific 'RealmAccess').
type HelixClaims struct {
	jwt.RegisteredClaims

	// User Identity
	Email string `json:"email,omitempty"`

	// Authorization
	// Atlas will issue roles in a flat array or space-separated scope.
	Roles []string `json:"roles,omitempty"`
	Scope string   `json:"scope,omitempty"`
}

// GetRoles provides a unified way to retrieve roles.
func (c *HelixClaims) GetRoles() []string {
	// If Atlas uses "scope" string (standard OAuth2), we might split it here later.
	// For now, we return the explicit Roles array.
	if c.Roles == nil {
		return []string{}
	}
	return c.Roles
}
