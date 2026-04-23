package domain

import (
	"encoding/json"
	"errors"
	"fmt"
)

type ErrorKind uint8

const (
	KindValidation ErrorKind = iota
	KindNotFound
	KindUnauthorized
	KindConflict
	KindForbidden
)

type DomainError interface {
	Code() string
	Kind() ErrorKind
}

type Error struct {
	Message string
	Details any
	code    string
	kind    ErrorKind
}

func (e *Error) Error() string {
	errStr := e.Message

	if e.Details != nil {
		// Use JSON to flatten "any" complex structure into a readable string
		j, err := json.Marshal(e.Details)
		if err == nil {
			return fmt.Sprintf("%s (details: %s)", errStr, string(j))
		}
		// Final fallback for edge cases where JSON fails
		return fmt.Sprintf("%s (details: %v)", errStr, e.Details)
	}

	return errStr
}

func (e *Error) Is(target error) bool {
	var t DomainError
	if !errors.As(target, &t) {
		return false
	}
	return e.Kind() == t.Kind() && e.Code() == t.Code()
}

func (e *Error) Code() string {
	return e.code
}

func (e *Error) Kind() ErrorKind {
	return e.kind
}

// WithDetails returns a new Error instance with the provided details.
// The original Error is not modified. The returned error remains comparable
// to the original via errors.Is.
func (e *Error) WithDetails(details any) *Error {
	c := *e // shallow copy
	if details != nil {
		c.Details = details
	}
	return &c
}

// SetMessage returns a copy with Message replaced.
func (e *Error) SetMessage(message string) *Error {
	c := *e
	c.Message = message
	return &c
}

// Error constructors.
var (
	UnauthorizedErr = constructErr(KindUnauthorized, "UNAUTHORIZED")
	ConflictErr     = constructErr(KindConflict, "CONFLICT")
	ForbiddenErr    = constructErr(KindForbidden, "FORBIDDEN")
	ValidationErr   = constructErr(KindValidation, "DOMAIN_VALIDATION")
)

// No constructor for NotFound

var ErrNotFound = constructErr(KindNotFound, "RESOURCE_NOT_FOUND")("the requested resource was not found")

func constructErr(kind ErrorKind, defaultCode string) func(message string, code ...string) *Error {
	return func(msg string, code ...string) *Error {
		errCode := defaultCode
		if len(code) > 0 && code[0] != "" {
			errCode = code[0]
		}
		return &Error{
			Message: msg,
			Details: nil,
			code:    errCode,
			kind:    kind,
		}
	}
}
