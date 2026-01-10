package crypto

import "github.com/golang-jwt/jwt/v5"

type HelixClaims struct {
	jwt.RegisteredClaims

	Scope string   `json:"scope,omitempty"`
	Roles []string `json:"roles,omitempty"`
	Sid   string   `json:"sid,omitempty"`
}
