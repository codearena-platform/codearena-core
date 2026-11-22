package realtime

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/codearena-platform/codearena-core/internal/security"
	"github.com/golang-jwt/jwt/v5"
)

func TestValidateToken(t *testing.T) {
	secret := "test-secret"
	t.Setenv("JWT_SECRET", secret)
	t.Setenv("JWT_ISSUER", "")
	t.Setenv("JWT_AUDIENCE", "")

	sign := func(claims jwt.MapClaims) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		raw, err := token.SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("failed to sign token: %v", err)
		}
		return raw
	}
	validClaims := func() jwt.MapClaims {
		return jwt.MapClaims{
			"exp": time.Now().Add(time.Hour).Unix(),
			"iss": security.DefaultIssuer,
			"aud": security.DefaultAudience,
		}
	}

	t.Run("Valid Token", func(t *testing.T) {
		parsed, err := validateToken(sign(validClaims()))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !parsed.Valid {
			t.Error("expected token to be valid")
		}
	})

	t.Run("Expired Token", func(t *testing.T) {
		claims := validClaims()
		claims["exp"] = time.Now().Add(-time.Hour).Unix()
		parsed, err := validateToken(sign(claims))
		if err == nil {
			t.Error("expected error for expired token, got nil")
		}
		if parsed != nil && parsed.Valid {
			t.Error("expected token to be invalid")
		}
	})

	t.Run("Missing Issuer", func(t *testing.T) {
		claims := validClaims()
		delete(claims, "iss")
		if _, err := validateToken(sign(claims)); err == nil {
			t.Error("expected error for token without issuer")
		}
	})

	t.Run("Wrong Audience", func(t *testing.T) {
		claims := validClaims()
		claims["aud"] = "someone-else"
		if _, err := validateToken(sign(claims)); err == nil {
			t.Error("expected error for token with wrong audience")
		}
	})

	t.Run("Invalid Secret", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims())
		tokenString, _ := token.SignedString([]byte("wrong-secret"))

		parsed, err := validateToken(tokenString)
		if err == nil {
			t.Error("expected error for wrong secret, got nil")
		}
		if parsed != nil && parsed.Valid {
			t.Error("expected token to be invalid")
		}
	})

	t.Run("Invalid Algorithm", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS512, validClaims())
		tokenString, _ := token.SignedString([]byte(secret))
		if _, err := validateToken(tokenString); err == nil {
			t.Error("expected HS512 token to be rejected")
		}
	})

	t.Run("Missing Secret", func(t *testing.T) {
		t.Setenv("JWT_SECRET", "")
		if _, err := validateToken(sign(validClaims())); err == nil {
			t.Error("expected error when JWT_SECRET is not configured")
		}
	})
}

func TestServeWsRejectsQueryToken(t *testing.T) {
	// Tokens in URLs are rejected before any WebSocket upgrade is attempted.
	t.Setenv("JWT_SECRET", "test-secret")
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ws/match/test?token=valid-looking-token", nil)
	serveWs(newHub(nil), recorder, req, "test")
	if recorder.Code != 401 {
		t.Fatalf("expected 401 for query-string token, got %d", recorder.Code)
	}
}
