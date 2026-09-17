package http

import (
	"github.com/gofiber/fiber/v3"
	"github.com/prawirdani/golang-restapi/internal/ports/repository"
	"github.com/prawirdani/golang-restapi/pkg/validator"
)

// MaxBodySize maximum read size from request body
const MaxBodySize = 5 << 20 // 5 MB

// Body is the JSON envelope for every successful response: the payload under
// "data", an optional human-readable "message", and "meta" for paginated
// collections.
//
// The envelope is sparse: Data and Message carry omitempty and Meta carries
// omitzero, so only fields holding a value are emitted and an empty envelope
// marshals to "{}".
//
// Caveat on Data: it is typed [any], so omitempty drops it only while the field
// is left unset. A typed nil — a nil slice, map, or pointer stored in the
// interface — is not empty, and still marshals as "data":null. Leave Data unset
// rather than assigning a typed nil when there is nothing to return.
//
// Meta is a value tagged omitzero, not a pointer tagged omitempty: omitempty
// never omits a struct, so a value field using it would be emitted as all
// zeroes on every response. It is omitted only when genuinely zero, which a
// paginated list never is — Pagination always applies a default limit.
//
// Errors use a separate envelope; see [Error] and [ParseError].
type Body struct {
	Data    any                       `json:"data,omitempty"`
	Message string                    `json:"message,omitempty"`
	Meta    repository.PaginationMeta `json:"meta,omitzero"`
}

func BindValidateJSON(c fiber.Ctx, dst any) error {
	if err := c.Bind().JSON(&dst); err != nil {
		return parseJSONBindErr(err)
	}
	return validator.Validate(dst)
}
