package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/prawirdani/golang-restapi/internal/audit"
)

type auditRepository struct {
	db *DB
}

func NewAuditRepository(db *DB) *auditRepository {
	return &auditRepository{db: db}
}

// Record implements [audit.Recorder]. It reuses the transactional connection
// when one is present in ctx, so the audit row commits atomically with the
// action that produced it. Ambient request metadata (IP, user-agent) is merged
// with any per-entry Meta into a single JSONB meta column.
func (r *auditRepository) Record(ctx context.Context, e audit.Entry) error {
	prev, err := marshalPayload(e.Prev)
	if err != nil {
		return fmt.Errorf("marshal audit prev: %w", err)
	}
	next, err := marshalPayload(e.Next)
	if err != nil {
		return fmt.Errorf("marshal audit next: %w", err)
	}
	meta, err := marshalMeta(ctx, e.Meta)
	if err != nil {
		return fmt.Errorf("marshal audit meta: %w", err)
	}

	var actorID any // nil -> NULL for system actions
	var role any
	if actor := audit.ActorID(ctx); actor != nil {
		role = string(actor.Role)
		if actor.UserID != nil {
			actorID = *actor.UserID
		}
	}

	const query = `
		INSERT INTO audit_logs
			(actor_id, actor_role, action, entity, entity_id, prev, next, meta)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	conn := r.db.GetConn(ctx)
	if _, err := conn.Exec(
		ctx, query,
		actorID, role, string(e.Action), e.Entity, e.EntityID, prev, next, meta,
	); err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

// marshalMeta merges ambient request metadata with per-entry Meta. The entry's
// Meta fields win on conflict (e.g. an unauthenticated login records its own
// target email alongside the middleware-captured IP/user-agent).
func marshalMeta(ctx context.Context, entryMeta any) ([]byte, error) {
	base := audit.RequestMetaFromContext(ctx)
	if entryMeta == nil {
		return marshalPayload(base)
	}

	merged := map[string]any{}
	b, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &merged); err != nil {
		return nil, err
	}

	mb, err := json.Marshal(entryMeta)
	if err != nil {
		return nil, err
	}
	var entry map[string]any
	if err := json.Unmarshal(mb, &entry); err != nil {
		// entryMeta is not a JSON object; keep it as a scalar under "value".
		return json.Marshal(map[string]any{"request": base, "value": entryMeta})
	}
	maps.Copy(merged, entry)
	return json.Marshal(merged)
}

// marshalPayload returns nil (SQL NULL) for a nil payload, otherwise JSON bytes.
func marshalPayload(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
