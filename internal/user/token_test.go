package user_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"zhu/internal/user"
)

func TestGenerateAndParseToken(t *testing.T) {
	token, err := user.GenerateToken("test-secret", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claims, err := user.ParseToken("test-secret", token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if claims.UserID != 42 {
		t.Fatalf("expected user id 42, got %d", claims.UserID)
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	token, err := user.GenerateToken("test-secret", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := user.ParseToken("other-secret", token); err == nil {
		t.Fatal("expected error when parsing token with wrong secret")
	}
}

func TestParseToken_Expired(t *testing.T) {
	claims := user.Claims{
		UserID: 42,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := user.ParseToken("test-secret", signed); err == nil {
		t.Fatal("expected error when parsing expired token")
	}
}
