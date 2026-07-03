package user_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"pandabk_coin/internal/user"
	"gorm.io/gorm"
)

const testSecret = "test-secret"

func setupRouter(t *testing.T) (*gin.Engine, *user.Service) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&user.User{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	svc := &user.Service{DB: db}
	h := &user.Handler{Service: svc, JWTSecret: testSecret}

	router := gin.New()
	api := router.Group("/api")
	user.RegisterRoutes(api, h)

	return router, svc
}

func TestLogin_ReturnsToken(t *testing.T) {
	router, svc := setupRouter(t)

	if _, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"email": "alice@example.com", "password": "password123"})
	req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected a non-empty token in login response")
	}
}

func TestMe_WithoutToken(t *testing.T) {
	router, _ := setupRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestMe_WithInvalidToken(t *testing.T) {
	router, _ := setupRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestMe_WithValidToken(t *testing.T) {
	router, svc := setupRouter(t)

	created, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	token, err := user.GenerateToken(testSecret, created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var got user.User
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if got.Email != "alice@example.com" {
		t.Fatalf("expected email %q, got %q", "alice@example.com", got.Email)
	}
}
