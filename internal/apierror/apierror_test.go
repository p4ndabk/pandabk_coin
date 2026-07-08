package apierror_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"zhu/internal/apierror"
)

func respond(err error) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	apierror.Respond(c, err)
	return w
}

func TestRespond_AppError(t *testing.T) {
	w := respond(apierror.Conflict("email_taken", "email already in use"))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, w.Code)
	}

	var body apierror.Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if body.Error.Code != "email_taken" || body.Error.Message != "email already in use" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestRespond_RecordNotFound(t *testing.T) {
	w := respond(gorm.ErrRecordNotFound)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}

	var body apierror.Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("expected code %q, got %q", "not_found", body.Error.Code)
	}
}

func TestRespond_UnexpectedError_HidesMessage(t *testing.T) {
	sensitive := errors.New("UNIQUE constraint failed: users.email, driver internals leaked")
	w := respond(sensitive)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	var body apierror.Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if body.Error.Code != "internal_error" {
		t.Fatalf("expected code %q, got %q", "internal_error", body.Error.Code)
	}
	if body.Error.Message != "internal server error" {
		t.Fatalf("expected generic message, got %q", body.Error.Message)
	}
	if got := w.Body.String(); strings.Contains(got, "UNIQUE constraint") {
		t.Fatalf("response leaked internal error detail: %s", got)
	}
}
