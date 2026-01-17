package response

import "net/http"

const (
	// General & System
	ErrSystem         = "SYS_INTERNAL_ERROR"
	ErrBadRequest     = "SYS_BAD_REQUEST"
	ErrServiceUnavail = "SYS_SERVICE_UNAVAILABLE"
	ErrGatewayTimeout = "SYS_GATEWAY_TIMEOUT"

	// Validation
	ErrValidation    = "VAL_INVALID_INPUT"
	ErrMissingField  = "VAL_MISSING_FIELD"
	ErrInvalidFormat = "VAL_INVALID_FORMAT"

	// Auth
	ErrMissingToken  = "AUTH_MISSING_TOKEN"
	ErrInvalidToken  = "AUTH_INVALID_TOKEN"
	ErrForbidden     = "AUTH_FORBIDDEN"
	ErrAccountLocked = "AUTH_ACCOUNT_LOCKED"

	// Resource / Data (Database Mapped)
	ErrNotFound        = "RES_NOT_FOUND"
	ErrAlreadyExists   = "RES_ALREADY_EXISTS"
	ErrConflict        = "RES_CONFLICT"
	ErrVersionMismatch = "RES_VERSION_MISMATCH"

	// Business Logic
	ErrRuleViolation = "BIZ_RULE_VIOLATION"
	ErrRateLimit     = "BIZ_RATE_LIMIT_EXCEEDED"
)

func MapStatus(code string) int {
	switch code {
	case ErrBadRequest, ErrValidation, ErrMissingField, ErrInvalidFormat:
		return http.StatusBadRequest

	case ErrMissingToken, ErrInvalidToken:
		return http.StatusUnauthorized

	case ErrForbidden, ErrAccountLocked:
		return http.StatusForbidden

	case ErrNotFound:
		return http.StatusNotFound

	case ErrAlreadyExists, ErrConflict, ErrVersionMismatch:
		return http.StatusConflict

	case ErrRateLimit:
		return http.StatusTooManyRequests

	case ErrRuleViolation:
		return http.StatusUnprocessableEntity

	case ErrServiceUnavail, ErrGatewayTimeout:
		return http.StatusServiceUnavailable

	case ErrSystem:
		fallthrough
	default:
		return http.StatusInternalServerError
	}
}
