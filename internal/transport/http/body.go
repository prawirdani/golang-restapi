package http

import (
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/prawirdani/golang-restapi/pkg/validator"
)

// MaxBodySize maximum read size from request body
const MaxBodySize = 5 << 20 // 5 MB

// Body is json response body
type Body struct {
	Data    any    `json:"data"`
	Message string `json:"message"`
}

// MarshalJSON implements [json.Marshaller] to keep the "message" field present (as null
// when empty) without forcing a pointer on the struct field.
func (b *Body) MarshalJSON() ([]byte, error) {
	// Alias avoids infinite recursion into this MarshalJSON; *string lets us
	// emit null for an empty message while keeping the field present.
	type alias struct {
		Data    any     `json:"data"`
		Message *string `json:"message"`
	}

	a := alias{Data: b.Data}
	if b.Message != "" {
		a.Message = &b.Message
	}

	return json.Marshal(a)
}

func BindValidateJSON(c fiber.Ctx, dst any) error {
	if err := c.Bind().JSON(&dst); err != nil {
		return parseJSONBindErr(err)
	}
	return validator.Validate(dst)
}
