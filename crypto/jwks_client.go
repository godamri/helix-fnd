package crypto

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"
)

type JWKSVerifier interface {
	VerifyToken(tokenString string) (*HelixClaims, error)
}

type jwks struct {
	Keys []jsonWebKey `json:"keys"`
}

type jsonWebKey struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type CachingClient struct {
	jwksURL          string
	issuer           string
	cache            map[string]*rsa.PublicKey
	lastUpdated      time.Time
	maxStaleDuration time.Duration
	mu               sync.RWMutex
	log              *slog.Logger
	client           *http.Client
	sf               singleflight.Group
}

func NewJWKSCachingClient(ctx context.Context, jwksURL string, issuer string, refreshInterval time.Duration, maxStaleDuration time.Duration, logger *slog.Logger) (JWKSVerifier, error) {
	if jwksURL == "" || issuer == "" {
		return nil, errors.New("jwks client: URL and Issuer are mandatory")
	}

	if maxStaleDuration <= 0 {
		maxStaleDuration = 24 * time.Hour
	}

	c := &CachingClient{
		jwksURL:          jwksURL,
		issuer:           issuer,
		cache:            make(map[string]*rsa.PublicKey),
		maxStaleDuration: maxStaleDuration,
		log:              logger.With("component", "JWKSClient"),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	if err := c.fetchKeys(ctx); err != nil {
		return nil, fmt.Errorf("jwks client: FATAL on initial key fetch: %w", err)
	}

	go c.startKeyRefresher(ctx, refreshInterval)

	return c, nil
}

func (c *CachingClient) startKeyRefresher(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	c.log.Info("JWKS Caching Client background refresher started",
		"interval", interval.String(),
		"url", c.jwksURL,
	)

	for {
		select {
		case <-ctx.Done():
			c.log.Info("JWKS Caching Client refresher shutting down...")
			return
		case <-ticker.C:
			refreshCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := c.doRefresh(refreshCtx); err != nil {
				c.mu.RLock()
				age := time.Since(c.lastUpdated)
				c.mu.RUnlock()

				c.log.Error("Failed to auto-refresh JWKS keys",
					"error", err,
					"cache_age", age.String(),
				)
			}
			cancel()
		}
	}
}

func (c *CachingClient) doRefresh(ctx context.Context) error {
	_, err, _ := c.sf.Do("refresh_keys", func() (interface{}, error) {
		return nil, c.fetchKeys(ctx)
	})
	return err
}

func (c *CachingClient) fetchKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("JWKS endpoint returned status %d", resp.StatusCode)
	}

	var newJwks jwks
	if err := json.NewDecoder(resp.Body).Decode(&newJwks); err != nil {
		return fmt.Errorf("failed to decode JWKS response: %w", err)
	}

	newCache := make(map[string]*rsa.PublicKey)
	for _, jwk := range newJwks.Keys {
		if jwk.Kty != "RSA" || jwk.Use != "sig" || jwk.Kid == "" {
			continue
		}
		key, err := jwk.toRSAPublicKey()
		if err != nil {
			c.log.Warn("Skipping invalid JWK", "kid", jwk.Kid, "error", err)
			continue
		}
		newCache[jwk.Kid] = key
	}

	if len(newCache) == 0 {
		return errors.New("JWKS response contains zero valid RSA keys")
	}

	c.mu.Lock()
	c.cache = newCache
	c.lastUpdated = time.Now()
	c.mu.Unlock()

	return nil
}

func (j *jsonWebKey) toRSAPublicKey() (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		return nil, fmt.Errorf("invalid modulus (n): %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)

	eBytes, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		return nil, fmt.Errorf("invalid exponent (e): %w", err)
	}

	eVal := 0
	for _, b := range eBytes {
		eVal = (eVal << 8) | int(b)
	}

	return &rsa.PublicKey{N: n, E: eVal}, nil
}

var (
	ErrInvalidToken = errors.New("crypto: invalid token")
	ErrExpiredToken = errors.New("crypto: token expired")
)

func (c *CachingClient) VerifyToken(tokenString string) (*HelixClaims, error) {
	c.mu.RLock()
	lastUpd := c.lastUpdated
	c.mu.RUnlock()

	if time.Since(lastUpd) > c.maxStaleDuration {
		c.log.Error("CRITICAL: JWKS cache is stale beyond limit",
			"age", time.Since(lastUpd).String(),
			"limit", c.maxStaleDuration.String(),
		)
	}

	token, err := jwt.ParseWithClaims(tokenString, &HelixClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("missing kid in token header")
		}

		c.mu.RLock()
		key, found := c.cache[kid]
		c.mu.RUnlock()

		if found {
			return key, nil
		}

		c.log.Info("Key ID miss, attempting emergency refresh...", "kid", kid)
		refreshCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := c.doRefresh(refreshCtx); err != nil {
			return nil, fmt.Errorf("failed emergency key refresh: %w", err)
		}

		c.mu.RLock()
		key, found = c.cache[kid]
		c.mu.RUnlock()

		if !found {
			return nil, fmt.Errorf("kid %s not found in JWKS even after emergency refresh", kid)
		}

		return key, nil
	},
		jwt.WithIssuer(c.issuer),
		jwt.WithExpirationRequired(),
	)

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*HelixClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	now := time.Now()
	if exp, err := claims.GetExpirationTime(); err == nil {
		if now.Add(-1 * time.Minute).After(exp.Time) {
			return nil, ErrExpiredToken
		}
	}

	if nbf, err := claims.GetNotBefore(); err == nil && nbf != nil {
		if now.Add(1 * time.Minute).Before(nbf.Time) {
			return nil, fmt.Errorf("%w: token not active yet", ErrInvalidToken)
		}
	}

	return claims, nil
}
