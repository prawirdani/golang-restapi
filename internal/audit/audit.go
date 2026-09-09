// Package audit records user-initiated state changes (who changed what, from
// which previous state to which next state) for compliance and forensics.
package audit

import (
	"context"

	"github.com/prawirdani/golang-restapi/internal/rbac"
)

// Action names a recorded event using the shared authorization grammar:
// "<entity>.<verb>[-<object>]" (lowercase; dot separates entity from verb;
// hyphens join a multiword verb). It intentionally mirrors [rbac.Permission]:
// an Action's coarse verb is the permission that gates it (N:1), e.g. both
// "user.update" and "user.change-profile-picture" are gated by "user.update".
// Define Action constants beside the entity's Permission constants so the two
// layers stay visibly aligned.
type Action string

// RequestMeta is ambient HTTP request metadata captured once by middleware and
// carried in the context. The Recorder merges it into every entry it writes.
type RequestMeta struct {
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
}

// Entry is a single audit record. Prev and Next are arbitrary payloads
// serialized to JSONB by the Recorder implementation; nil means "no state"
// (e.g. Prev is nil on create, Next is nil on delete). Meta carries
// per-entry context not covered by the ambient RequestMeta (e.g. the target
// email of an unauthenticated login attempt).
type Entry struct {
	Action   Action // event name, e.g. "user.change-profile-picture"
	Entity   string // logical entity name (indexed), e.g. "user"
	EntityID string // affected entity's identifier
	Prev     any    // state before the change (nil if not applicable)
	Next     any    // state after the change (nil if not applicable)
	Meta     any    // per-entry metadata (nil if none)
}

// Recorder persists audit entries. Implementations resolve the acting principal
// and ambient request metadata from the context (see [rbac.GetContext] and
// [RequestMetaFromContext]); system actions have a nil actor. When called
// inside a transaction, the write must join that transaction so the action and
// its audit record commit or roll back together. For best-effort records (e.g.
// failed actions with no transaction), callers pass a non-tx context.
type Recorder interface {
	Record(ctx context.Context, e Entry) error
}

// ActorID returns the acting user's ID from context, or nil for system/worker
// actions (no rbac context). Exposed so implementations share one resolution rule.
func ActorID(ctx context.Context) *rbac.Actor {
	rbacCtx, err := rbac.GetContext(ctx)
	if err != nil {
		return nil
	}
	return &rbacCtx.Actor
}

type metaCtxKey struct{}

// WithRequestMeta stores ambient request metadata in ctx.
func WithRequestMeta(ctx context.Context, m RequestMeta) context.Context {
	return context.WithValue(ctx, metaCtxKey{}, m)
}

// RequestMetaFromContext returns the ambient request metadata, or zero if none.
func RequestMetaFromContext(ctx context.Context) RequestMeta {
	m, _ := ctx.Value(metaCtxKey{}).(RequestMeta)
	return m
}
