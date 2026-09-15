// Package audit records user-initiated state changes (who changed what, from
// which previous state to which next state) for compliance and forensics.
package audit

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/pkg/nullable"
)

// Action names a recorded event using the shared authorization grammar:
// "<entity>.<verb>[-<object>]" (lowercase; dot separates entity from verb;
// hyphens join a multiword verb). It intentionally mirrors [rbac.Permission]:
// an Action's coarse verb is the permission that gates it (N:1), e.g. both
// "user.update" and "user.change-profile-picture" are gated by "user.update".
// Define Action constants beside the entity's Permission constants so the two
// layers stay visibly aligned.
type Action string

// Entry is a single audit record. Prev and Next are arbitrary payloads
// serialized to JSONB by the Recorder implementation; nil means "no state"
// (e.g. Prev is nil on create, Next is nil on delete).
type Entry struct {
	ID        int                          `db:"id"         json:"id"`
	ActorID   nullable.Nullable[uuid.UUID] `db:"actor_id"   json:"actor_id"`  // nil if executed by system
	Action    Action                       `db:"action"     json:"action"`    // event name, e.g. "user.change-profile-picture"
	Entity    string                       `db:"entity"     json:"entity"`    // logical entity name (indexed), e.g. "user"
	EntityID  string                       `db:"entity_id"  json:"entity_id"` // affected entity's identifier
	Prev      any                          `db:"prev"       json:"prev"`      // state before the change (nil if not applicable)
	Next      any                          `db:"next"       json:"next"`      // state after the change (nil if not applicable)
	Meta      map[string]any               `db:"meta"       json:"meta"`      // per-entry metadata (nil if none)
	CreatedAt time.Time                    `db:"created_at" json:"created_at"`
}

// FillActorMeta fills ActorID field and Meta.
func (e *Entry) FillActorMeta(ctx context.Context) {
	if e.Meta == nil {
		e.Meta = make(map[string]any)
	}
	if v, _ := GetContext(ctx); v != nil {
		e.Meta["request_id"] = v.RequestID
		e.Meta["ip_addr"] = v.IP
		e.Meta["user_agent"] = v.UserAgent
	}

	if v, _ := rbac.GetContext(ctx); v != nil {
		if v.Actor.UserID != nil {
			e.ActorID.Set(*v.Actor.UserID, false)
			e.Meta["session_id"] = v.SessionID
		}
		e.Meta["actor_role"] = v.Actor.Role // System role doesn't rely on active session.
	}
}

// Recorder persists audit entries.
type Recorder interface {
	Record(ctx context.Context, e Entry) error
}

type Reader interface {
	List(ctx context.Context) ([]Entry, error)
}
