package dockerapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Sentinel errors returned by request calls; use errors.Is to check.
var (
	// ErrNotFound is returned for a 404 response.
	ErrNotFound = errors.New("dockerapi: not found")
	// ErrConflict is returned for a 409 response (e.g. already running/stopped).
	ErrConflict = errors.New("dockerapi: conflict")
	// ErrNotModified is returned for a 304 response.
	ErrNotModified = errors.New("dockerapi: not modified")
	// ErrUnreachable is returned when the socket/TCP dial itself fails.
	ErrUnreachable = errors.New("dockerapi: engine unreachable")
)

// APIError is returned for any other non-2xx response.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("dockerapi: %d: %s", e.Status, e.Message)
}

// dockerErrorBody mirrors Docker's {"message":"..."} error envelope.
type dockerErrorBody struct {
	Message string `json:"message"`
}

// mapError inspects a non-2xx *http.Response and returns the matching
// sentinel or an *APIError. It consumes and closes resp.Body.
func mapError(resp *http.Response) error {
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	msg := string(body)
	var parsed dockerErrorBody
	if json.Unmarshal(body, &parsed) == nil && parsed.Message != "" {
		msg = parsed.Message
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, msg)
	case http.StatusConflict:
		return fmt.Errorf("%w: %s", ErrConflict, msg)
	case http.StatusNotModified:
		return ErrNotModified
	default:
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
}
