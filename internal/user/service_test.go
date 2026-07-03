package user_test

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"pandabk_coin/internal/user"
	"gorm.io/gorm"
)

func newTestService(t *testing.T) *user.Service {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	if err := db.AutoMigrate(&user.User{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return &user.Service{DB: db}
}

func TestService_Create(t *testing.T) {
	svc := newTestService(t)

	u, err := svc.Create(user.CreateInput{
		Name:     "Alice",
		Email:    "alice@example.com",
		Password: "password123",
		Active:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if u.ID == 0 {
		t.Fatal("expected user to have an ID after creation")
	}

	if u.Password == "password123" {
		t.Fatal("expected password to be hashed, got plaintext")
	}
}

func TestService_Create_DuplicateEmail(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := svc.Create(user.CreateInput{Name: "Alice 2", Email: "alice@example.com", Password: "password123", Active: true})
	if !errors.Is(err, user.ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestService_List(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.Create(user.CreateInput{Name: "Bob", Email: "bob@example.com", Password: "password123", Active: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	users, err := svc.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
}

func TestService_GetByID(t *testing.T) {
	svc := newTestService(t)

	created, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found, err := svc.GetByID(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if found.Email != "alice@example.com" {
		t.Fatalf("expected email %q, got %q", "alice@example.com", found.Email)
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.GetByID(999)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected gorm.ErrRecordNotFound, got %v", err)
	}
}

func TestService_Update(t *testing.T) {
	svc := newTestService(t)

	created, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, err := svc.Update(created.ID, user.UpdateInput{
		Name:   "Alice Updated",
		Email:  "alice2@example.com",
		Active: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updated.Name != "Alice Updated" || updated.Email != "alice2@example.com" || updated.Active {
		t.Fatalf("update did not apply expected changes: %+v", updated)
	}

	if updated.Password == "" {
		t.Fatal("expected password to be preserved when not provided in update")
	}
}

func TestService_Update_DuplicateEmail(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bob, err := svc.Create(user.CreateInput{Name: "Bob", Email: "bob@example.com", Password: "password123", Active: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.Update(bob.ID, user.UpdateInput{Name: "Bob", Email: "alice@example.com", Active: true})
	if !errors.Is(err, user.ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestService_Delete(t *testing.T) {
	svc := newTestService(t)

	created, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := svc.Delete(created.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = svc.GetByID(created.ID)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected user to be deleted, got err %v", err)
	}
}

func TestService_Authenticate_Success(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	u, err := svc.Authenticate("alice@example.com", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if u.Email != "alice@example.com" {
		t.Fatalf("expected email %q, got %q", "alice@example.com", u.Email)
	}
}

func TestService_Authenticate_WrongPassword(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := svc.Authenticate("alice@example.com", "wrong-password")
	if !errors.Is(err, user.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestService_Authenticate_InactiveUser(t *testing.T) {
	svc := newTestService(t)

	if _, err := svc.Create(user.CreateInput{Name: "Alice", Email: "alice@example.com", Password: "password123", Active: false}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := svc.Authenticate("alice@example.com", "password123")
	if !errors.Is(err, user.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for inactive user, got %v", err)
	}
}

func TestService_Authenticate_UnknownEmail(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Authenticate("nobody@example.com", "password123")
	if !errors.Is(err, user.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}
