package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/prawirdani/golang-restapi/internal/domain"
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
	Value  any    `json:"value"`
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

func ErrInvalidParam(name string, value any) *Error {
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

func NormalizeError(err error) *Error {
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
		maxBytesErr   *http.MaxBytesError
		jsonBindErr   *JSONBindError
		validationErr *validator.ValidationError
		domainErr     *domain.Error
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

	case errors.As(err, &maxBytesErr):
		return ErrBodyTooLarge.SetDetails(map[string]int{"max_bytes": int(maxBytesErr.Limit)})
	case errors.As(err, &jsonBindErr):
		body.status = http.StatusBadRequest
		body.Message = jsonBindErr.Message
		body.Code = "REQ_MALFORMED_JSON"
	case errors.As(err, &validationErr):
		body.status = http.StatusUnprocessableEntity
		body.Message = "the request contains invalid data"
		body.Details = validationErr.Details
		body.Code = "VALIDATION"
	case errors.As(err, &domainErr):
		body.status = getDomainErrStatusCode(domainErr.Kind())
		body.Message = domainErr.Message
		body.Details = domainErr.Details
		body.Code = domainErr.Code()
	}

	return body
}

type JSONBindError struct {
	Message string
}

func (e *JSONBindError) Error() string {
	return e.Message
}

func ParseJSONBindErr(err error) error {
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

	return &JSONBindError{Message: msg}
}

func IsMissingFileError(err error) bool {
	if errors.Is(err, http.ErrMissingFile) || errors.Is(err, io.EOF) {
		return true
	}

	return false
}

var domainErrStatusMap = map[domain.ErrorKind]int{
	domain.KindNotFound:     http.StatusNotFound,
	domain.KindValidation:   http.StatusUnprocessableEntity,
	domain.KindConflict:     http.StatusConflict,
	domain.KindForbidden:    http.StatusForbidden,
	domain.KindUnauthorized: http.StatusUnauthorized,
}

func getDomainErrStatusCode(kind domain.ErrorKind) int {
	if status, ok := domainErrStatusMap[kind]; ok {
		return status
	}
	return http.StatusInternalServerError
}
