package crypto

import "github.com/golang-jwt/jwt/v5"

type HelixClaims struct {
	jwt.RegisteredClaims
	Email     string   `json:"email"`
	Roles     []string `json:"roles"`
	Scope     string   `json:"scope,omitempty"`
	OrgID     string   `json:"org_id,omitempty"`
	ActorType string   `json:"actor_type,omitempty"`
}

func (c *HelixClaims) GetRoles() []string {
	if c.Roles == nil {
		return []string{}
	}
	return c.Roles
}

func (c *HelixClaims) GetActorType() string {
	if c.ActorType == "" {
		return "human"
	}
	return c.ActorType
}
