package throttle

import (
	"context"
	"time"
)

// Throttler restricts how often an operation identified by a key may
// proceed within a given time window. Implementations must be safe for
// concurrent use by multiple goroutines and, if backed by a shared store
// such as Redis or mutexes map, by multiple processes or instances.
type Throttler interface {
	// TryAcquire attempts to reserve a slot for key. If no slot is currently
	// held, it acquires one that expires after ttl and returns a Result with
	// Allowed set to true. If a slot is already held for key, it returns a
	// Result with Allowed set to false and RetryAfter set to the remaining
	// time until that slot expires.
	//
	// A non-nil error indicates the underlying check itself failed (e.g. a
	// connectivity error with the backing store), not a throttle decision.
	// Callers should treat such errors as "throttle state unknown" rather
	// than as an implicit allow or deny.
	TryAcquire(ctx context.Context, key string, ttl time.Duration) (Result, error)

	// Release clears any active slot held for key, so the next TryAcquire
	// call for that key succeeds immediately regardless of the original ttl.
	// Release is best-effort: implementations may log failures rather than
	// return them, and callers should not depend on it succeeding, since a
	// failed release only leaves the key throttled until its ttl expires
	// naturally.
	Release(ctx context.Context, key string) error
}
