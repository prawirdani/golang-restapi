package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// MaxBodySize maximum read size from request body
const MaxBodySize = 5 << 20 // 5 MB

// Body is json response body
type Body struct {
	Data    any    `json:"data"`
	Message string `json:"message"`
}

// MarshalJSON implements json.Marshaller to prevent using pointer on Body fields while preserving the field on the body
// with nullable ability
func (b *Body) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"data": b.Data,
	}

	if b.Message != "" {
		m["message"] = b.Message
	} else {
		m["message"] = nil
	}

	return json.Marshal(m)
}

// eTag generate strong etag from given data
func eTag(data any) string {
	b, err := json.Marshal(data)
	if err != nil {
		return ""
	}

	h := sha256.Sum256(b)
	return fmt.Sprintf(`"%s"`, hex.EncodeToString(h[:]))
}
