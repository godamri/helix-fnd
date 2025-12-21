package crypto

import "github.com/golang-jwt/jwt/v5"

type HelixClaims struct {
	jwt.RegisteredClaims
	Email string   `json:"email"`
	Roles []string `json:"roles"`
	Scope string   `json:"scope,omitempty"`
}

// GetRoles returns the roles safely.
func (c *HelixClaims) GetRoles() []string {
	if c.Roles == nil {
		return []string{}
	}
	return c.Roles
}
