package audit

import (
	"context"
	"errors"
	"net"
)

var ErrCtxNotFound = errors.New("audit context not found")

type Context struct {
	IP        net.IP
	RequestID string
	UserAgent string
}

type ctxKey struct{}

var key ctxKey

// WithContext stores the Audit Context to context.
func WithContext(ctx context.Context, value Context) context.Context {
	return context.WithValue(ctx, key, value)
}

// GetContext retrieves the audit context from the context.
func GetContext(ctx context.Context) (*Context, error) {
	value, ok := ctx.Value(key).(Context)
	if !ok {
		return nil, ErrCtxNotFound
	}
	return &value, nil
}
