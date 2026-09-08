package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/0funct0ry/vessel/internal/dockerapi"
	"github.com/0funct0ry/vessel/internal/store"
)

type errorBody struct {
	Code         string     `json:"code"`
	Message      string     `json:"message"`
	DockerStatus int        `json:"docker_status,omitempty"`
	Required     store.Role `json:"required,omitempty"`
	Actual       store.Role `json:"actual,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type queryError struct {
	message string
}

type validationError struct{ code, message string }

func (e *validationError) Error() string { return e.message }
func invalidInput(format string, args ...any) error {
	return &validationError{code: "invalid_request", message: fmt.Sprintf(format, args...)}
}
func invalidName(format string, args ...any) error {
	return &validationError{code: "invalid_name", message: fmt.Sprintf(format, args...)}
}

func (e *queryError) Error() string { return e.message }

func invalidQuery(format string, args ...any) error {
	return &queryError{message: fmt.Sprintf(format, args...)}
}

type resourceError struct {
	resource string
	id       string
	err      error
}

func (e *resourceError) Error() string { return e.err.Error() }
func (e *resourceError) Unwrap() error { return e.err }

func forResource(resource, id string, err error) error {
	if err == nil {
		return nil
	}
	return &resourceError{resource: resource, id: id, err: err}
}

// Fail writes the stable API error envelope for an application or Docker
// client error. Handlers return immediately after calling it.
func Fail(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	body := errorBody{Code: "internal_error", Message: "internal server error"}

	var qerr *queryError
	var verr *validationError
	var rerr *resourceError
	var apiErr *dockerapi.APIError
	switch {
	case errors.As(err, &qerr):
		status = http.StatusBadRequest
		body = errorBody{Code: "invalid_query", Message: qerr.message}
	case errors.As(err, &verr):
		status = http.StatusBadRequest
		body = errorBody{Code: verr.code, Message: verr.message}
	case errors.Is(err, dockerapi.ErrUnreachable):
		status = http.StatusServiceUnavailable
		body = errorBody{Code: "docker_unreachable", Message: "Docker Engine is unreachable"}
	case errors.Is(err, dockerapi.ErrNotFound):
		status = http.StatusNotFound
		body.DockerStatus = http.StatusNotFound
		if errors.As(err, &rerr) {
			body.Code = rerr.resource + "_not_found"
			body.Message = fmt.Sprintf("no such %s: %s", rerr.resource, rerr.id)
		} else {
			body.Code = "docker_not_found"
			body.Message = dockerMessage(err)
		}
	case errors.Is(err, dockerapi.ErrConflict):
		status = http.StatusConflict
		body = errorBody{Code: "already_in_state", Message: dockerMessage(err), DockerStatus: http.StatusConflict}
	case errors.Is(err, dockerapi.ErrNotModified):
		c.Status(http.StatusNotModified)
		return
	case errors.As(err, &apiErr):
		status = apiErr.Status
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		body = errorBody{Code: "docker_error", Message: apiErr.Message, DockerStatus: apiErr.Status}
	}

	c.AbortWithStatusJSON(status, errorEnvelope{Error: body})
}

func dockerMessage(err error) string {
	message := err.Error()
	if i := strings.LastIndex(message, ": "); i >= 0 && i+2 < len(message) {
		return message[i+2:]
	}
	return message
}
