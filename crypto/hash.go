package crypto

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

type HashConfig struct {
	Cost int `envconfig:"BCRYPT_COST" default:"12"`
}

type Hasher struct {
	cost int
}

func NewHasher(cfg HashConfig) *Hasher {
	cost := cfg.Cost
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = 12
	}
	return &Hasher{cost: cost}
}

func (h *Hasher) HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("crypto: password cannot be empty")
	}

	bytes, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to hash password: %w", err)
	}
	return string(bytes), nil
}

func CheckPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
