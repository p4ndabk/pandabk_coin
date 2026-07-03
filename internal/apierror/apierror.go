// Package apierror centralizes how domain/service errors become HTTP
// responses: a shared JSON envelope, and a single place that decides
// whether an error's real message is safe to expose to the client.
package apierror

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AppError carries the HTTP status and machine-readable code a handler
// wants in the response. Err, when set, is the underlying cause: it is
// logged server-side but never serialized to the client.
type AppError struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

func New(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}

func BadRequest(code, message string) *AppError {
	return New(http.StatusBadRequest, code, message)
}

func Unauthorized(code, message string) *AppError {
	return New(http.StatusUnauthorized, code, message)
}

func NotFound(code, message string) *AppError {
	return New(http.StatusNotFound, code, message)
}

func Conflict(code, message string) *AppError {
	return New(http.StatusConflict, code, message)
}

// Detail is the JSON shape of a single error.
type Detail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Body is the JSON envelope every error response shares — also the type
// referenced by Swagger @Failure annotations.
type Body struct {
	Error Detail `json:"error"`
}

// Respond writes err as a JSON error response:
//   - *AppError: uses its Status/Code/Message as-is
//   - gorm.ErrRecordNotFound: 404 "not_found"
//   - anything else: treated as unexpected — logged server-side with the
//     real error, client gets a generic 500 "internal_error" (never leaks
//     internal details like driver/SQL error text)
func Respond(c *gin.Context, err error) {
	var appErr *AppError
	switch {
	case errors.As(err, &appErr):
		if appErr.Status >= http.StatusInternalServerError && appErr.Err != nil {
			slog.Error("internal error", "error", appErr.Err)
		}
		c.JSON(appErr.Status, Body{Error: Detail{Code: appErr.Code, Message: appErr.Message}})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, Body{Error: Detail{Code: "not_found", Message: "resource not found"}})
	default:
		slog.Error("internal error", "error", err)
		c.JSON(http.StatusInternalServerError, Body{Error: Detail{Code: "internal_error", Message: "internal server error"}})
	}
}
