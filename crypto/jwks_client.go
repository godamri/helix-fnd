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

func NewJWKSCachingClient(jwksURL string, issuer string, refreshInterval time.Duration, maxStaleDuration time.Duration, logger *slog.Logger) (JWKSVerifier, error) {
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

	// Fail-fast on startup
	if err := c.fetchKeys(context.Background()); err != nil {
		return nil, fmt.Errorf("jwks client: FATAL on initial key fetch: %w", err)
	}

	go c.startKeyRefresher(context.Background(), refreshInterval)

	return c, nil
}

func (c *CachingClient) startKeyRefresher(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	c.log.Info("JWKS Caching Client started",
		"interval", interval.String(),
		"stale_limit", c.maxStaleDuration.String(),
		"url", c.jwksURL,
	)

	for {
		select {
		case <-ctx.Done():
			c.log.Info("JWKS Caching Client shutting down.")
			return
		case <-ticker.C:
			refreshCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := c.doRefresh(refreshCtx); err != nil {
				timeSinceUpdate := time.Since(c.lastUpdated)
				c.log.Error("Failed to refresh JWKS keys",
					"error", err,
					"cache_age", timeSinceUpdate.String(),
				)
			} else {
				c.log.Debug("JWKS keys refreshed successfully")
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
		if jwk.Kty != "RSA" || jwk.Use != "sig" {
			continue
		}
		if jwk.Kid == "" {
			continue
		}
		key, err := jwk.toRSAPublicKey()
		if err != nil {
			c.log.Warn("Failed to convert JWK", "kid", jwk.Kid, "error", err)
			continue
		}
		newCache[jwk.Kid] = key
	}

	if len(newCache) == 0 {
		return errors.New("JWKS response contains zero valid RSA keys for signature")
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

	if len(eBytes) == 0 {
		return nil, errors.New("invalid exponent (e): empty bytes")
	}

	eVal := 0
	for _, b := range eBytes {
		eVal = (eVal << 8) | int(b)
	}

	if eVal == 0 {
		return nil, errors.New("invalid exponent (e): value is zero")
	}

	return &rsa.PublicKey{N: n, E: eVal}, nil
}

var (
	ErrInvalidToken = errors.New("crypto: invalid token")
	ErrExpiredToken = errors.New("crypto: token expired")
)

// VerifyToken validates the JWT using cached keys.
func (c *CachingClient) VerifyToken(tokenString string) (*HelixClaims, error) {
	// Audit Stale Cache (Non-blocking check)
	c.mu.RLock()
	cacheAge := time.Since(c.lastUpdated)
	c.mu.RUnlock()

	if cacheAge > c.maxStaleDuration {
		c.log.Error("CRITICAL: JWKS keys are stale. Auth Provider unreachable?",
			"age", cacheAge.String(),
			"limit", c.maxStaleDuration.String(),
			"action", "using_stale_cache_to_maintain_availability",
		)
	}

	// Parse & Verify in one go (Avoids double parsing)
	token, err := jwt.ParseWithClaims(tokenString, &HelixClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate Algo
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		// Extract Key ID
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("missing or invalid kid in header")
		}

		// Cache Lookup
		c.mu.RLock()
		key, found := c.cache[kid]
		c.mu.RUnlock()

		if found {
			return key, nil
		}

		// Cache Miss? Try Refresh (Singleflight protected)
		c.log.Info("Key ID not found in cache. Attempting refresh...", "kid", kid)
		if err := c.doRefresh(context.Background()); err != nil {
			// If refresh fails, we can't find the key
			return nil, fmt.Errorf("failed key refresh: %w", err)
		}

		// Retry Lookup after refresh
		c.mu.RLock()
		key, found = c.cache[kid]
		c.mu.RUnlock()

		if !found {
			return nil, fmt.Errorf("kid %s not found in JWKS even after refresh", kid)
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

	if claims, ok := token.Claims.(*HelixClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrInvalidToken
}
