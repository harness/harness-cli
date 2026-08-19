package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// APIError is a non-2xx response from a Harness service.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Code       string
	Message    string
	Body       string
}

func (e *APIError) Error() string {
	detail := e.Message
	if detail == "" {
		detail = e.Body
	}
	if detail == "" {
		detail = http.StatusText(e.StatusCode)
	}

	msg := fmt.Sprintf("%s %s failed with HTTP %d: %s", e.Method, e.Path, e.StatusCode, detail)
	if hint := e.hint(); hint != "" {
		msg += "\n  " + hint
	}

	return msg
}

func (e *APIError) hint() string {
	switch e.StatusCode {
	case http.StatusUnauthorized:
		return "The API key was rejected. Check that it is valid and not expired."
	case http.StatusForbidden:
		return "The API key is valid but lacks permission for this operation. " +
			"Load test operations need the load_loadtest_view, load_loadtest_edit " +
			"or load_loadtest_delete permission on the target scope."
	case http.StatusNotFound:
		return "Check the identifier, and that --account, --org and --project point at the right scope."
	}
	return ""
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.StatusCode == http.StatusNotFound
}

// IsConflict reports whether the object already exists. A duplicate identity
// arrives as a 500 quoting the driver error, not a 409, so the text is matched.
func IsConflict(err error) bool {
	apiErr, ok := err.(*APIError)
	if !ok {
		return false
	}
	if apiErr.StatusCode == http.StatusConflict {
		return true
	}
	return strings.Contains(apiErr.Body, duplicateKeyMarker) ||
		strings.Contains(apiErr.Message, duplicateKeyMarker)
}

// duplicateKeyMarker is MongoDB's duplicate key error code, as it appears in
// an unwrapped driver error.
const duplicateKeyMarker = "E11000 duplicate key"

// errorPayload matches the loadTestManager ErrorResponse shape. Other Harness
// services use "code"/"message", so both spellings are accepted.
type errorPayload struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func newAPIError(method, path string, statusCode int, body []byte) *APIError {
	apiErr := &APIError{
		Method:     method,
		Path:       path,
		StatusCode: statusCode,
		Body:       strings.TrimSpace(string(body)),
	}

	var payload errorPayload
	if err := json.Unmarshal(body, &payload); err == nil {
		apiErr.Message = payload.Message
		apiErr.Code = payload.Error
		if apiErr.Code == "" {
			apiErr.Code = payload.Code
		}

		// loadTestManager puts the readable reason in "error" and sends no
		// "message"; without this the raw JSON document gets printed.
		if apiErr.Message == "" && payload.Error != "" {
			apiErr.Message = payload.Error
		}
	}

	return apiErr
}
