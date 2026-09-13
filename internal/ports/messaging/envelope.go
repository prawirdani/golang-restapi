package messaging

import (
	"time"

	"github.com/google/uuid"
)

type Envelope[T any] struct {
	ID        string    `json:"id"`        // idempotency key
	Timestamp time.Time `json:"timestamp"` // when produced, not when consumed
	Payload   T         `json:"payload"`
}

func NewEnvelope[T any](data T) Envelope[T] {
	return Envelope[T]{
		ID:        uuid.New().String(),
		Timestamp: time.Now().UTC(),
		Payload:   data,
	}
}
