package security

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "websocket-test-secret"

func signedToken(t *testing.T, method jwt.SigningMethod, secret string, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return token
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
		"iss": DefaultIssuer,
		"aud": DefaultAudience,
	}
}

func TestValidateTokenWithConfig(t *testing.T) {
	cfg := JWTConfig{Secret: testSecret}

	t.Run("valid HS256 token", func(t *testing.T) {
		token, err := ValidateTokenWithConfig(signedToken(t, jwt.SigningMethodHS256, testSecret, validClaims()), cfg)
		if err != nil || token == nil || !token.Valid {
			t.Fatalf("expected valid token, got token=%v err=%v", token, err)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		claims := validClaims()
		claims["exp"] = time.Now().Add(-time.Hour).Unix()
		if _, err := ValidateTokenWithConfig(signedToken(t, jwt.SigningMethodHS256, testSecret, claims), cfg); err == nil {
			t.Error("expected expired token to be rejected")
		}
	})

	t.Run("missing expiration", func(t *testing.T) {
		claims := validClaims()
		delete(claims, "exp")
		if _, err := ValidateTokenWithConfig(signedToken(t, jwt.SigningMethodHS256, testSecret, claims), cfg); err == nil {
			t.Error("expected token without exp to be rejected")
		}
	})

	t.Run("wrong issuer", func(t *testing.T) {
		claims := validClaims()
		claims["iss"] = "attacker"
		if _, err := ValidateTokenWithConfig(signedToken(t, jwt.SigningMethodHS256, testSecret, claims), cfg); err == nil {
			t.Error("expected token with wrong issuer to be rejected")
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		claims := validClaims()
		claims["aud"] = "other-service"
		if _, err := ValidateTokenWithConfig(signedToken(t, jwt.SigningMethodHS256, testSecret, claims), cfg); err == nil {
			t.Error("expected token with wrong audience to be rejected")
		}
	})

	t.Run("wrong secret", func(t *testing.T) {
		if _, err := ValidateTokenWithConfig(signedToken(t, jwt.SigningMethodHS256, "wrong", validClaims()), cfg); err == nil {
			t.Error("expected token signed with wrong secret to be rejected")
		}
	})

	t.Run("HS512 rejected", func(t *testing.T) {
		if _, err := ValidateTokenWithConfig(signedToken(t, jwt.SigningMethodHS512, testSecret, validClaims()), cfg); err == nil {
			t.Error("expected non-HS256 algorithm to be rejected")
		}
	})

	t.Run("alg none rejected", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims())
		raw, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatalf("failed to create alg=none token: %v", err)
		}
		if _, err := ValidateTokenWithConfig(raw, cfg); err == nil {
			t.Error("expected alg=none token to be rejected")
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		if _, err := ValidateTokenWithConfig(signedToken(t, jwt.SigningMethodHS256, testSecret, validClaims()), JWTConfig{}); err == nil {
			t.Error("expected missing secret to be rejected")
		}
	})
}

func TestTokenFromRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/ws?token=from-query", nil)
	if _, err := TokenFromRequest(req); err == nil {
		t.Error("expected missing Authorization header to be rejected")
	}
	req.Header.Set("Authorization", "Bearer header-token")
	if token, err := TokenFromRequest(req); err != nil || token != "header-token" {
		t.Fatalf("expected header token, got %q err=%v", token, err)
	}
	req.Header.Set("Authorization", "Basic header-token")
	if _, err := TokenFromRequest(req); err == nil {
		t.Error("expected non-Bearer scheme to be rejected")
	}
}

func TestAllowedOriginsFromEnv(t *testing.T) {
	t.Setenv("WS_ALLOWED_ORIGINS", " https://arena.example.com , https://admin.example.com/ ")
	got := AllowedOriginsFromEnv()
	if len(got) != 2 || got[0] != "https://arena.example.com" || got[1] != "https://admin.example.com" {
		t.Fatalf("unexpected allowlist: %v", got)
	}
}

func TestOriginAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Host = "arena.example.com"
	allowed := []string{"https://trusted.example.com"}

	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{"non-browser client", "", true},
		{"allowlisted origin", "https://trusted.example.com", true},
		{"allowlisted origin with path", "https://trusted.example.com/app", true},
		{"same origin", "https://arena.example.com", true},
		{"foreign origin", "https://evil.example.com", false},
		{"scheme mismatch for allowlisted host", "http://trusted.example.com", false},
		{"malformed origin", "not-a-url", false},
		{"javascript scheme", "javascript:alert(1)", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OriginAllowed(tt.origin, req, allowed); got != tt.want {
				t.Fatalf("OriginAllowed(%q)=%v, want %v", tt.origin, got, tt.want)
			}
		})
	}

	if !CheckOrigin(allowed)(req) {
		t.Error("CheckOrigin should allow a request without Origin header")
	}
}
