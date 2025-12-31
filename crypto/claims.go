package crypto

import "github.com/golang-jwt/jwt/v5"

type HelixClaims struct {
	jwt.RegisteredClaims
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	OrgID       string   `json:"org_id,omitempty"`
	ActorType   string   `json:"actor_type,omitempty"`
}

func (c *HelixClaims) GetRoles() []string {
	if c.Roles == nil {
		return []string{}
	}
	return c.Roles
}

func (c *HelixClaims) GetPermissions() []string {
	if c.Permissions == nil {
		return []string{}
	}
	return c.Permissions
}

func (c *HelixClaims) GetActorType() string {
	if c.ActorType == "" {
		return "human"
	}
	return c.ActorType
}

func (c *HelixClaims) GetOrgID() string {
	return c.OrgID
}

func (c *HelixClaims) GetEmail() string {
	return c.Email
}
