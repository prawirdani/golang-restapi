package validator

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// FieldError represents a single field validation error
type FieldError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
	Value   string `json:"value,omitempty"`
}

// ValidationError represents validation errors for multiple fields
type ValidationError struct {
	Errors  []FieldError        `json:"errors"`
	Details map[string][]string `json:"details"`
}

// Error implements the error interface
func (v *ValidationError) Error() string {
	if len(v.Errors) == 0 {
		return "validation failed"
	}

	var msgs []string
	for field, errs := range v.Details {
		msgs = append(msgs, fmt.Sprintf("%s: %s", field, strings.Join(errs, ", ")))
	}
	return strings.Join(msgs, "; ")
}

func (v *ValidationError) Merge(vErr *ValidationError) {
	if vErr == nil {
		return
	}

	if len(vErr.Errors) > 0 {
		v.Errors = append(v.Errors, vErr.Errors...)
	}

	if len(vErr.Details) > 0 {
		if v.Details == nil {
			v.Details = make(map[string][]string)
		}
		for field, messages := range vErr.Details {
			v.Details[field] = append(v.Details[field], messages...)
		}
	}
}

// HasField checks if a specific field has validation errors
func (v *ValidationError) HasField(field string) bool {
	_, exists := v.Details[field]
	return exists
}

// GetField returns all error messages for a specific field
func (v *ValidationError) GetField(field string) []string {
	return v.Details[field]
}

// JSON returns the JSON representation of the validation error
func (v *ValidationError) JSON() ([]byte, error) {
	return json.Marshal(v.Details)
}

// Fields returns all field names that have errors
func (v *ValidationError) Fields() []string {
	fields := make([]string, 0, len(v.Details))
	for field := range v.Details {
		fields = append(fields, field)
	}
	return fields
}

// convertError converts validator.ValidationErrors to custom ValidationError
func convertError(errs validator.ValidationErrors) *ValidationError {
	fieldErrors := make([]FieldError, 0, len(errs))
	details := make(map[string][]string)

	for _, e := range errs {
		field := e.Field()
		msg := buildErrorMessage(e)

		fieldError := FieldError{
			Field:   field,
			Tag:     e.Tag(),
			Message: msg,
			Value:   fmt.Sprintf("%v", e.Value()),
		}

		fieldErrors = append(fieldErrors, fieldError)
		details[field] = append(details[field], msg)
	}

	return &ValidationError{
		Errors:  fieldErrors,
		Details: details,
	}
}

// buildErrorMessage creates a human-readable error message based on the validation tag
func buildErrorMessage(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "this field is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return buildMinMessage(e)
	case "max":
		return buildMaxMessage(e)
	case "len":
		return fmt.Sprintf("must be exactly %s characters", e.Param())
	case "gt":
		return fmt.Sprintf("must be greater than %s", e.Param())
	case "gte":
		return fmt.Sprintf("must be greater than or equal to %s", e.Param())
	case "lt":
		return fmt.Sprintf("must be less than %s", e.Param())
	case "lte":
		return fmt.Sprintf("must be less than or equal to %s", e.Param())
	case "alpha":
		return "must contain only letters"
	case "alphanum":
		return "must contain only letters and numbers"
	case "numeric":
		return "must be a numeric value"
	case "url":
		return "must be a valid URL"
	case "uri":
		return "must be a valid URI"
	case "contains":
		return fmt.Sprintf("must contain '%s'", e.Param())
	case "containsany":
		return fmt.Sprintf("must contain at least one of: %s", e.Param())
	case "excludes":
		return fmt.Sprintf("must not contain '%s'", e.Param())
	case "startswith":
		return fmt.Sprintf("must start with '%s'", e.Param())
	case "endswith":
		return fmt.Sprintf("must end with '%s'", e.Param())
	case "oneof":
		return fmt.Sprintf("must be one of: %s", e.Param())
	case "uuid":
		return "must be a valid UUID"
	case "datetime":
		return fmt.Sprintf("must be a valid datetime in format: %s", e.Param())
	default:
		return fmt.Sprintf("validation failed on '%s' tag", e.Tag())
	}
}

// buildMinMessage creates error message for min validation
func buildMinMessage(e validator.FieldError) string {
	switch e.Kind() {
	case reflect.String:
		return fmt.Sprintf("must be at least %s characters long", e.Param())
	case reflect.Slice, reflect.Array, reflect.Map:
		return fmt.Sprintf("must contain at least %s items", e.Param())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return fmt.Sprintf("must be at least %s", e.Param())
	default:
		return fmt.Sprintf("must be at least %s", e.Param())
	}
}

// buildMaxMessage creates error message for max validation
func buildMaxMessage(e validator.FieldError) string {
	switch e.Kind() {
	case reflect.String:
		return fmt.Sprintf("must be at most %s characters long", e.Param())
	case reflect.Slice, reflect.Array, reflect.Map:
		return fmt.Sprintf("must contain at most %s items", e.Param())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return fmt.Sprintf("must be at most %s", e.Param())
	default:
		return fmt.Sprintf("must be at most %s", e.Param())
	}
}
