// Package hub provides the Scion Hub API server.
package hub

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/ptone/scion-agent/pkg/store"
)

// APIError represents a standardized error response.
type APIError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	RequestID string                 `json:"requestId,omitempty"`
}

// ErrorResponse wraps an APIError for JSON responses.
type ErrorResponse struct {
	Error APIError `json:"error"`
}

// Error codes matching the Hub API specification.
const (
	ErrCodeInvalidRequest     = "invalid_request"
	ErrCodeValidationError    = "validation_error"
	ErrCodeUnauthorized       = "unauthorized"
	ErrCodeForbidden          = "forbidden"
	ErrCodeNotFound           = "not_found"
	ErrCodeConflict           = "conflict"
	ErrCodeVersionConflict    = "version_conflict"
	ErrCodeUnprocessable      = "unprocessable"
	ErrCodeRateLimited        = "rate_limited"
	ErrCodeInternalError      = "internal_error"
	ErrCodeRuntimeError       = "runtime_error"
	ErrCodeUnavailable        = "unavailable"
	ErrCodeNoRuntimeHost      = "no_runtime_host"
	ErrCodeRuntimeHostUnavail = "runtime_host_unavailable"

	// Host authentication error codes
	ErrCodeInvalidJoinToken = "invalid_join_token"
	ErrCodeExpiredJoinToken = "expired_join_token"
	ErrCodeHostAuthFailed   = "host_auth_failed"
	ErrCodeInvalidSignature = "invalid_signature"
	ErrCodeClockSkew        = "clock_skew"
	ErrCodeReplayDetected   = "replay_detected"
)

// writeError writes a JSON error response.
// For 5xx errors, it logs the error details for debugging.
func writeError(w http.ResponseWriter, statusCode int, code, message string, details map[string]interface{}) {
	// Log 5xx errors for debugging
	if statusCode >= 500 {
		log.Printf("[Hub] ERROR %d: %s - %s", statusCode, code, message)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := ErrorResponse{
		Error: APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// writeErrorFromErr writes an error response based on a Go error.
// For 5xx errors, it logs the underlying error for debugging.
func writeErrorFromErr(w http.ResponseWriter, err error, requestID string) {
	var statusCode int
	var code, message string

	switch {
	case errors.Is(err, store.ErrNotFound):
		statusCode = http.StatusNotFound
		code = ErrCodeNotFound
		message = "Resource not found"
	case errors.Is(err, store.ErrAlreadyExists):
		statusCode = http.StatusConflict
		code = ErrCodeConflict
		message = "Resource already exists"
	case errors.Is(err, store.ErrVersionConflict):
		statusCode = http.StatusConflict
		code = ErrCodeVersionConflict
		message = "Version conflict - resource was modified"
	case errors.Is(err, store.ErrInvalidInput):
		statusCode = http.StatusBadRequest
		code = ErrCodeValidationError
		message = "Invalid input"
	default:
		statusCode = http.StatusInternalServerError
		code = ErrCodeInternalError
		message = "Internal server error"
	}

	// Log 5xx errors with the underlying error for debugging
	if statusCode >= 500 {
		log.Printf("[Hub] ERROR %d: %s - %v", statusCode, code, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	resp := ErrorResponse{
		Error: APIError{
			Code:      code,
			Message:   message,
			RequestID: requestID,
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// NotFound writes a 404 Not Found response.
func NotFound(w http.ResponseWriter, resource string) {
	writeError(w, http.StatusNotFound, ErrCodeNotFound,
		resource+" not found", nil)
}

// BadRequest writes a 400 Bad Request response.
func BadRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, ErrCodeInvalidRequest, message, nil)
}

// ValidationError writes a 400 Bad Request response for validation failures.
func ValidationError(w http.ResponseWriter, message string, details map[string]interface{}) {
	writeError(w, http.StatusBadRequest, ErrCodeValidationError, message, details)
}

// Unauthorized writes a 401 Unauthorized response.
func Unauthorized(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, ErrCodeUnauthorized,
		"Authentication required", nil)
}

// Forbidden writes a 403 Forbidden response.
func Forbidden(w http.ResponseWriter) {
	writeError(w, http.StatusForbidden, ErrCodeForbidden,
		"Insufficient permissions", nil)
}

// Conflict writes a 409 Conflict response.
func Conflict(w http.ResponseWriter, message string) {
	writeError(w, http.StatusConflict, ErrCodeConflict, message, nil)
}

// InternalError writes a 500 Internal Server Error response.
func InternalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, ErrCodeInternalError,
		"Internal server error", nil)
}

// MethodNotAllowed writes a 405 Method Not Allowed response.
func MethodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed",
		"Method not allowed", nil)
}

// RuntimeError writes a 502 Bad Gateway response for runtime host errors.
func RuntimeError(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadGateway, ErrCodeRuntimeError, message, nil)
}

// GatewayTimeout writes a 504 Gateway Timeout response for runtime host timeouts.
func GatewayTimeout(w http.ResponseWriter, message string) {
	writeError(w, http.StatusGatewayTimeout, ErrCodeUnavailable, message, nil)
}

// NoRuntimeHost writes a 422 Unprocessable Entity response when no runtime host
// is available for agent creation. Includes available hosts as alternatives.
func NoRuntimeHost(w http.ResponseWriter, message string, availableHosts []RuntimeHostSummary) {
	details := map[string]interface{}{
		"availableHosts": availableHosts,
	}
	writeError(w, http.StatusUnprocessableEntity, ErrCodeNoRuntimeHost, message, details)
}

// RuntimeHostUnavailable writes a 503 Service Unavailable response when the
// specified runtime host is not available.
func RuntimeHostUnavailable(w http.ResponseWriter, hostID string, availableHosts []RuntimeHostSummary) {
	details := map[string]interface{}{
		"requestedHostId": hostID,
		"availableHosts":  availableHosts,
	}
	writeError(w, http.StatusServiceUnavailable, ErrCodeRuntimeHostUnavail,
		"Specified runtime host is unavailable", details)
}

// RuntimeHostSummary is a minimal representation of a runtime host for error responses.
type RuntimeHostSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}
