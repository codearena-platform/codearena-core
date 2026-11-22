// Package security contains the shared JWT and WebSocket checks used by the
// engine dashboard and the realtime service.
package security

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const (
	DefaultIssuer   = "codearena"
	DefaultAudience = "codearena-clients"
)

// JWTConfig describes the claims required on WebSocket access tokens.
type JWTConfig struct {
	Secret   string
	Issuer   string
	Audience string
}

// JWTConfigFromEnv loads JWT settings. Issuer and audience have safe defaults
// but are always validated; a missing secret rejects every token.
func JWTConfigFromEnv() JWTConfig {
	cfg := JWTConfig{
		Secret:   strings.TrimSpace(os.Getenv("JWT_SECRET")),
		Issuer:   strings.TrimSpace(os.Getenv("JWT_ISSUER")),
		Audience: strings.TrimSpace(os.Getenv("JWT_AUDIENCE")),
	}
	if cfg.Issuer == "" {
		cfg.Issuer = DefaultIssuer
	}
	if cfg.Audience == "" {
		cfg.Audience = DefaultAudience
	}
	return cfg
}

// ValidateToken enforces HS256, expiration, issuer and audience for a JWT.
func ValidateToken(tokenString string) (*jwt.Token, error) {
	return ValidateTokenWithConfig(tokenString, JWTConfigFromEnv())
}

// ValidateTokenWithConfig validates a token against an explicit configuration.
func ValidateTokenWithConfig(tokenString string, cfg JWTConfig) (*jwt.Token, error) {
	if cfg.Secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is not set")
	}
	if cfg.Issuer == "" {
		cfg.Issuer = DefaultIssuer
	}
	if cfg.Audience == "" {
		cfg.Audience = DefaultAudience
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cfg.Secret), nil
	},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithAudience(cfg.Audience),
	)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return token, nil
}

// TokenFromRequest reads the access token from the Authorization header.
// Query-string tokens are deliberately not accepted: URLs end up in logs.
func TokenFromRequest(r *http.Request) (string, error) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", fmt.Errorf("missing or malformed Authorization header")
	}
	return parts[1], nil
}

// AllowedOriginsFromEnv returns the configured WebSocket origin allowlist.
func AllowedOriginsFromEnv() []string {
	for _, key := range []string{"WS_ALLOWED_ORIGINS", "REALTIME_ALLOWED_ORIGINS"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			origins := make([]string, 0)
			for _, origin := range strings.Split(value, ",") {
				origin = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(origin), "/"))
				if origin != "" {
					origins = append(origins, origin)
				}
			}
			return origins
		}
	}
	return nil
}

// CheckOrigin returns a gorilla/websocket CheckOrigin function using an
// allowlist plus same-origin fallback for browsers without explicit config.
func CheckOrigin(allowed []string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		return OriginAllowed(r.Header.Get("Origin"), r, allowed)
	}
}

// OriginAllowed allows non-browser clients without an Origin header. Browser
// origins must either be allowlisted or match the request host (same-origin).
func OriginAllowed(origin string, r *http.Request, allowed []string) bool {
	if origin == "" {
		return true
	}
	originURL, err := url.Parse(origin)
	if err != nil || originURL.Host == "" || originURL.User != nil ||
		(originURL.Scheme != "http" && originURL.Scheme != "https") {
		return false
	}
	normalized := strings.ToLower(originURL.Scheme + "://" + originURL.Host)

	for _, candidate := range allowed {
		candidateURL, err := url.Parse(candidate)
		if err != nil || candidateURL.Host == "" {
			continue
		}
		if strings.ToLower(candidateURL.Scheme+"://"+candidateURL.Host) == normalized {
			return true
		}
	}
	return r != nil && strings.EqualFold(r.Host, originURL.Host)
}
