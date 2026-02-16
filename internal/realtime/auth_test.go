package realtime

import (
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestValidateToken(t *testing.T) {
	secret := "test-secret"
	os.Setenv("JWT_SECRET", secret)
	defer os.Unsetenv("JWT_SECRET")

	t.Run("Valid Token", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		tokenString, _ := token.SignedString([]byte(secret))

		parsed, err := validateToken(tokenString)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !parsed.Valid {
			t.Error("expected token to be valid")
		}
	})

	t.Run("Expired Token", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"exp": time.Now().Add(-time.Hour).Unix(),
		})
		tokenString, _ := token.SignedString([]byte(secret))

		parsed, err := validateToken(tokenString)
		if err == nil {
			t.Error("expected error for expired token, got nil")
		}
		if parsed != nil && parsed.Valid {
			t.Error("expected token to be invalid")
		}
	})

	t.Run("Invalid Secret", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		tokenString, _ := token.SignedString([]byte("wrong-secret"))

		parsed, err := validateToken(tokenString)
		if err == nil {
			t.Error("expected error for wrong secret, got nil")
		}
		if parsed != nil && parsed.Valid {
			t.Error("expected token to be invalid")
		}
	})
}
