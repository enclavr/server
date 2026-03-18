package errors

import (
	"errors"
	"fmt"
	"net/http"
)

type Code string

const (
	ErrCodeInternal             Code = "INTERNAL_ERROR"
	ErrCodeNotFound             Code = "NOT_FOUND"
	ErrCodeUnauthorized         Code = "UNAUTHORIZED"
	ErrCodeForbidden            Code = "FORBIDDEN"
	ErrCodeBadRequest           Code = "BAD_REQUEST"
	ErrCodeConflict             Code = "CONFLICT"
	ErrCodeRateLimit            Code = "RATE_LIMITED"
	ErrCodeValidation           Code = "VALIDATION_ERROR"
	ErrCodeTooLarge             Code = "PAYLOAD_TOO_LARGE"
	ErrCodeUnavailable          Code = "SERVICE_UNAVAILABLE"
	ErrCodeTimeout              Code = "TIMEOUT"
	ErrCodeInvalidToken         Code = "INVALID_TOKEN"
	ErrCodeTokenExpired         Code = "TOKEN_EXPIRED"
	ErrCodeAccountLocked        Code = "ACCOUNT_LOCKED"
	ErrCodeAccountDisabled      Code = "ACCOUNT_DISABLED"
	ErrCodeEmailNotVerified     Code = "EMAIL_NOT_VERIFIED"
	ErrCodeInvalidCredentials   Code = "INVALID_CREDENTIALS"
	ErrCode2FARequired          Code = "TWO_FACTOR_REQUIRED"
	ErrCode2FAInvalid           Code = "TWO_FACTOR_INVALID"
	ErrCodeWebhookFailed        Code = "WEBHOOK_FAILED"
	ErrCodeDatabaseError        Code = "DATABASE_ERROR"
	ErrCodeCacheError           Code = "CACHE_ERROR"
	ErrCodeExternalServiceError Code = "EXTERNAL_SERVICE_ERROR"
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
	case ErrCodeTimeout:
		return http.StatusRequestTimeout
	case ErrCodeInvalidToken, ErrCodeTokenExpired:
		return http.StatusUnauthorized
	case ErrCodeAccountLocked, ErrCodeAccountDisabled:
		return http.StatusForbidden
	case ErrCodeEmailNotVerified:
		return http.StatusForbidden
	case ErrCodeInvalidCredentials:
		return http.StatusUnauthorized
	case ErrCode2FARequired, ErrCode2FAInvalid:
		return http.StatusUnauthorized
	case ErrCodeWebhookFailed:
		return http.StatusBadGateway
	case ErrCodeDatabaseError, ErrCodeCacheError, ErrCodeExternalServiceError:
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

func InvalidToken() *Error {
	return New(ErrCodeInvalidToken, "Invalid authentication token")
}

func TokenExpired() *Error {
	return New(ErrCodeTokenExpired, "Authentication token has expired")
}

func AccountLocked(reason string) *Error {
	return New(ErrCodeAccountLocked, fmt.Sprintf("Account is locked: %s", reason))
}

func AccountDisabled() *Error {
	return New(ErrCodeAccountDisabled, "Account has been disabled")
}

func EmailNotVerified() *Error {
	return New(ErrCodeEmailNotVerified, "Email address has not been verified")
}

func InvalidCredentials() *Error {
	return New(ErrCodeInvalidCredentials, "Invalid username or password")
}

func TwoFactorRequired() *Error {
	return New(ErrCode2FARequired, "Two-factor authentication is required")
}

func TwoFactorInvalid() *Error {
	return New(ErrCode2FAInvalid, "Invalid two-factor authentication code")
}

func WebhookFailed(err error) *Error {
	if err == nil {
		return New(ErrCodeWebhookFailed, "Webhook delivery failed")
	}
	return Wrap(err, ErrCodeWebhookFailed, "Webhook delivery failed")
}

func DatabaseError(err error) *Error {
	if err == nil {
		return New(ErrCodeDatabaseError, "A database error occurred")
	}
	return Wrap(err, ErrCodeDatabaseError, "A database error occurred")
}

func CacheError(err error) *Error {
	if err == nil {
		return New(ErrCodeCacheError, "A cache error occurred")
	}
	return Wrap(err, ErrCodeCacheError, "A cache error occurred")
}

func ExternalServiceError(service string, err error) *Error {
	if err == nil {
		return New(ErrCodeExternalServiceError, fmt.Sprintf("External service '%s' is unavailable", service))
	}
	return Wrap(err, ErrCodeExternalServiceError, fmt.Sprintf("External service '%s' error", service))
}

func Timeout() *Error {
	return New(ErrCodeTimeout, "The request timed out")
}

func RateLimitExceeded(limit string) *Error {
	return New(ErrCodeRateLimit, fmt.Sprintf("Rate limit exceeded: %s", limit))
}

const (
	ErrCodeCircuitOpen     Code = "CIRCUIT_OPEN"
	ErrCodeCircuitHalfOpen Code = "CIRCUIT_HALF_OPEN"
	ErrCodeCircuitFailed   Code = "CIRCUIT_FAILED"
	ErrCodeRetryExhausted  Code = "RETRY_EXHAUSTED"
	ErrCodeServiceDown     Code = "SERVICE_DOWN"
)

func CircuitOpen(service string) *Error {
	return New(ErrCodeCircuitOpen, fmt.Sprintf("Circuit breaker open for service: %s", service))
}

func CircuitHalfOpen(service string) *Error {
	return New(ErrCodeCircuitHalfOpen, fmt.Sprintf("Circuit breaker half-open for service: %s", service))
}

func CircuitFailed(service string, err error) *Error {
	if err == nil {
		return New(ErrCodeCircuitFailed, fmt.Sprintf("Circuit breaker failed for service: %s", service))
	}
	return Wrap(err, ErrCodeCircuitFailed, fmt.Sprintf("Circuit breaker failed for service: %s", service))
}

func RetryExhausted(err error) *Error {
	if err == nil {
		return New(ErrCodeRetryExhausted, "All retry attempts exhausted")
	}
	return Wrap(err, ErrCodeRetryExhausted, "All retry attempts exhausted")
}

func ServiceDown(service string) *Error {
	return New(ErrCodeServiceDown, fmt.Sprintf("Service unavailable: %s", service))
}

type ErrorWithContext struct {
	*Error
	Context map[string]interface{}
}

func (e *ErrorWithContext) WithContext(key string, value interface{}) *ErrorWithContext {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

func NewErrorWithContext(code Code, message string) *ErrorWithContext {
	return &ErrorWithContext{
		Error:   New(code, message),
		Context: make(map[string]interface{}),
	}
}

func WrapWithContext(err error, code Code, message string) *ErrorWithContext {
	return &ErrorWithContext{
		Error:   Wrap(err, code, message),
		Context: make(map[string]interface{}),
	}
}
