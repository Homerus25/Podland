package errors

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse represents the JSON response for errors
type ErrorResponse struct {
	RequestID string       `json:"requestId"`
	Error     ErrorDetails `json:"error"`
}

// ErrorDetails contains error details
type ErrorDetails struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// RespondWithError writes an error response to the HTTP response writer
func RespondWithError(w http.ResponseWriter, err *AppError, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Status)

	response := ErrorResponse{
		RequestID: requestID,
		Error: ErrorDetails{
			Message: err.Message,
			Code:    err.Code,
		},
	}

	// For 5xx errors, use generic message
	if err.Status >= 500 {
		response.Error.Message = "An unexpected error occurred"
		response.Error.Code = "INTERNAL_ERROR"
	}

	json.NewEncoder(w).Encode(response)
}

// RespondWithHTTPError writes a simple HTTP error response
func RespondWithHTTPError(w http.ResponseWriter, status int, code, message string, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := ErrorResponse{
		RequestID: requestID,
		Error: ErrorDetails{
			Message: message,
			Code:    code,
		},
	}

	json.NewEncoder(w).Encode(response)
}
