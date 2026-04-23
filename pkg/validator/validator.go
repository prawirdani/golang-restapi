package validator

import (
	"errors"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var v *validator.Validate

func init() {
	v = validator.New()

	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
}

func Struct(s any) error {
	if err := v.Struct(s); err != nil {
		var vErrs validator.ValidationErrors
		if errors.As(err, &vErrs) {
			return convertError(vErrs)
		}
		return err
	}

	return nil
}

// Validator is implemented by types that can verify their internal state
// against defined constraints and requirements.
// Validate ensures the object's properties meet all criteria for validity.
// It returns an error if the object is in an invalid or inconsistent state.
type Validator interface{ Validate() error }

// Sanitizer is implemented by types that can perform self-normalization.
// Sanitize transforms the object's data into a canonical or safe format,
// typically by cleaning, trimming, or reformatting its fields.
type Sanitizer interface{ Sanitize() error }

// Validate checks if the input implements the [Sanitizer] or [Validator] interfaces.
// It first invokes Sanitize() to clean or normalize the data; if this fails,
// the error is returned immediately. It then invokes Validate() to ensure
// the input meets business rules.
func Validate(input any) error {
	if s, ok := input.(Sanitizer); ok {
		if err := s.Sanitize(); err != nil {
			return err
		}
	}

	if v, ok := input.(Validator); ok {
		return v.Validate()
	}

	// fallback
	return Struct(input)
}
