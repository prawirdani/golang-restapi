package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/pkg/validator"
)

var (
	ErrMultipartForm = &Error{
		Message: "invalid multipart form",
		Code:    "MULTIPART_FORM",
		status:  http.StatusBadRequest,
	}

	ErrBodyTooLarge = &Error{
		Message: "request body too large",
		Code:    "BODY_TOO_LARGE",
		status:  http.StatusRequestEntityTooLarge,
	}

	ErrRateLimit = &Error{
		Message: "too many request try again latter",
		Code:    "REQ_RATE_LIMIT",
		status:  http.StatusTooManyRequests,
	}

	ErrReqUnauthorized = &Error{
		Message: "authentication required",
		Code:    "REQ_UNAUTHORIZED",
		status:  http.StatusUnauthorized,
	}

	ErrReqForbidden = &Error{
		Message: "access forbidden",
		Code:    "REQ_FORBIDDEN",
		status:  http.StatusForbidden,
	}

	ErrNotFoundHandler = &Error{
		Message: "the requested resource could not be found",
		Code:    "HANDLER_NOT_FOUND",
		status:  http.StatusNotFound,
	}

	ErrMethodNotAllowedHandler = &Error{
		Message: "the method is not allowed for the requested url",
		Code:    "HANDLER_METHOD_NOT_ALLOWED",
		status:  http.StatusMethodNotAllowed,
	}
)

type Error struct {
	Message string `json:"message"`
	Details any    `json:"details"`
	Code    string `json:"code"`
	status  int    `json:"-"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	return e.Message
}

// Status returns the HTTP status code.
func (e *Error) Status() int {
	return e.status
}

// SetMessage returns a copy with Message replaced.
func (e *Error) SetMessage(message string) *Error {
	c := *e
	c.Message = message
	return &c
}

// SetDetails returns a copy with Details replaced.
func (e *Error) SetDetails(details any) *Error {
	c := *e
	c.Details = details
	return &c
}

type QueryParamErrorItem struct {
	Param  string `json:"param"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

func QueryParamErr(items []QueryParamErrorItem) *Error {
	return &Error{
		Message: "invalid query parameters",
		Code:    "INVALID_QUERY_PARAMETERS",
		Details: items,
		status:  http.StatusBadRequest,
	}
}

func ErrInvalidParam(name string, value string) *Error {
	return &Error{
		Message: fmt.Sprintf("invalid value '%v' for parameter '%s'", value, name),
		Code:    "INVALID_PARAMETER",
		Details: map[string]any{
			"parameter": name,
			"value":     value,
		},
		status: http.StatusBadRequest,
	}
}

func ParseError(err error) *Error {
	// Already normalized
	if e, ok := err.(*Error); ok {
		return e
	}

	body := &Error{
		status:  http.StatusInternalServerError,
		Message: "an unexpected error occurred, try again later",
		Code:    "INTERNAL",
	}

	var (
		fiberErr      *fiber.Error
		jsonBindErr   *jsonBindError
		validationErr *validator.ValidationError
		appErr        *apperr.Error
	)

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		body.status = http.StatusGatewayTimeout
		body.Message = "the server took too long to respond"
		body.Code = "SERVER_TIMEOUT"
		return body

	case errors.Is(err, context.Canceled):
		body.status = 499 // Client Closed Request
		return body

	case errors.As(err, &fiberErr):
		if fiberErr.Code == fiber.StatusRequestEntityTooLarge {
			return ErrBodyTooLarge.SetDetails(map[string]int{"max_bytes": int(MaxBodySize)})
		}
		return body

	case errors.As(err, &jsonBindErr):
		body.status = http.StatusBadRequest
		body.Message = jsonBindErr.Message
		body.Code = "REQ_MALFORMED_JSON"
	case errors.As(err, &validationErr):
		body.status = http.StatusUnprocessableEntity
		body.Message = "the request contains invalid data"
		body.Details = validationErr.Details
		body.Code = "VALIDATION"
	case errors.As(err, &appErr):
		body.status = appErrStatusCode(appErr.Kind())
		body.Message = appErr.Message
		body.Details = appErr.Details
		body.Code = appErr.Code()
	}

	return body
}

type jsonBindError struct {
	Message string
}

func (e *jsonBindError) Error() string {
	return e.Message
}

func parseJSONBindErr(err error) error {
	var syntaxError *json.SyntaxError
	var unmarshalTypeError *json.UnmarshalTypeError

	var msg string

	switch {
	case errors.As(err, &syntaxError):
		msg = fmt.Sprintf(
			"Request body contains badly-formed JSON (at position %d)",
			syntaxError.Offset,
		)

	case errors.Is(err, io.ErrUnexpectedEOF):
		msg = "Request body contains badly-formed JSON"

	case errors.As(err, &unmarshalTypeError):
		msg = fmt.Sprintf(
			"Request body contains an invalid value for the %q field (at position %d)",
			unmarshalTypeError.Field,
			unmarshalTypeError.Offset,
		)

	case strings.HasPrefix(err.Error(), "json: unknown field "):
		fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
		msg = fmt.Sprintf("Request body contains unknown field %s", fieldName)

	case errors.Is(err, io.EOF):
		msg = "Request body must not be empty"

	default:
		return err
	}

	return &jsonBindError{Message: msg}
}

var appErrStatusMap = map[apperr.Kind]int{
	apperr.KindNotFound:     http.StatusNotFound,
	apperr.KindValidation:   http.StatusUnprocessableEntity,
	apperr.KindConflict:     http.StatusConflict,
	apperr.KindForbidden:    http.StatusForbidden,
	apperr.KindUnauthorized: http.StatusUnauthorized,
	apperr.KindThrottled:    http.StatusTooManyRequests,
}

func appErrStatusCode(kind apperr.Kind) int {
	if status, ok := appErrStatusMap[kind]; ok {
		return status
	}
	return http.StatusInternalServerError
}
