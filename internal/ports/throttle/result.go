package throttle

import "time"

// Result describes the outcome of a Throttler.TryAcquire call.
type Result struct {
	// Allowed reports whether the caller may proceed. If false, the caller
	// should wait at least RetryAfter before attempting again.
	Allowed    bool      `json:"allowed"`
	RetryAfter time.Time `json:"retry_after"`
}
