package rbac

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrCtxNotFound = errors.New("rbac context not found in context")

// Actor identifies the principal performing an operation. UserID is nil for
// system/worker actions (written as NULL actor_id in audit logs); RoleSystem
// marks those entries.
type Actor struct {
	UserID *uuid.UUID
	Role   Role
}

type ctxKey struct{}

var key ctxKey

type Context struct {
	Actor     Actor
	SessionID uuid.UUID
}

// WithContext stores the AuthzContext to context.
func WithContext(ctx context.Context, value Context) context.Context {
	return context.WithValue(ctx, key, value)
}

func SystemContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, key, Context{
		Actor: Actor{Role: RoleSystem},
	})
}

// GetContext retrieves the rbac context from the context.
// Returns [ErrCtxNotFound] when rbac context has been injected (e.g. unauthenticated flows).
func GetContext(ctx context.Context) (*Context, error) {
	value, ok := ctx.Value(key).(Context)
	if !ok {
		return nil, ErrCtxNotFound
	}
	return &value, nil
}
