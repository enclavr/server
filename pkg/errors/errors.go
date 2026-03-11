package errors

import (
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	ErrCodeInternal     Code = "INTERNAL_ERROR"
	ErrCodeNotFound     Code = "NOT_FOUND"
	ErrCodeUnauthorized Code = "UNAUTHORIZED"
	ErrCodeForbidden    Code = "FORBIDDEN"
	ErrCodeBadRequest   Code = "BAD_REQUEST"
	ErrCodeConflict     Code = "CONFLICT"
	ErrCodeRateLimit    Code = "RATE_LIMITED"
	ErrCodeValidation   Code = "VALIDATION_ERROR"
	ErrCodeTooLarge     Code = "PAYLOAD_TOO_LARGE"
	ErrCodeUnavailable  Code = "SERVICE_UNAVAILABLE"
)

type Error struct {
	Code    Code     `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
	Err     error    `json:"-"`
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func (e *Error) WithDetails(details ...string) *Error {
	e.Details = details
	return e
}

func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func Wrap(err error, code Code, message string) *Error {
	return &Error{Code: code, Message: message, Err: err}
}

func Is(err, target error) bool {
	return errors.Is(err, target)
}

func As(err error, target interface{}) bool {
	return errors.As(err, target)
}

func ToHTTPStatus(code Code) int {
	switch code {
	case ErrCodeNotFound:
		return http.StatusNotFound
	case ErrCodeUnauthorized:
		return http.StatusUnauthorized
	case ErrCodeForbidden:
		return http.StatusForbidden
	case ErrCodeBadRequest:
		return http.StatusBadRequest
	case ErrCodeConflict:
		return http.StatusConflict
	case ErrCodeRateLimit:
		return http.StatusTooManyRequests
	case ErrCodeValidation:
		return http.StatusUnprocessableEntity
	case ErrCodeTooLarge:
		return http.StatusRequestEntityTooLarge
	case ErrCodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationErrors []ValidationError

func (v ValidationErrors) Error() string {
	return "validation failed"
}

func NewValidationError(field, message string) ValidationError {
	return ValidationError{Field: field, Message: message}
}

func InvalidParam(field, message string) *Error {
	return &Error{
		Code:    ErrCodeValidation,
		Message: "Invalid parameter",
		Details: []string{fmt.Sprintf("%s: %s", field, message)},
	}
}

func MissingParam(field string) *Error {
	return &Error{
		Code:    ErrCodeValidation,
		Message: "Missing required parameter",
		Details: []string{fmt.Sprintf("field '%s' is required", field)},
	}
}

func NotFound(resource string) *Error {
	return New(ErrCodeNotFound, fmt.Sprintf("%s not found", resource))
}

func AlreadyExists(resource string) *Error {
	return New(ErrCodeConflict, fmt.Sprintf("%s already exists", resource))
}

func Unauthorized() *Error {
	return New(ErrCodeUnauthorized, "Authentication required")
}

func Forbidden(resource string) *Error {
	return New(ErrCodeForbidden, fmt.Sprintf("Access denied to %s", resource))
}

func Internal(err error) *Error {
	if err == nil {
		return New(ErrCodeInternal, "An internal error occurred")
	}
	return Wrap(err, ErrCodeInternal, "An internal error occurred")
}
