package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prawirdani/golang-restapi/internal/audit"
)

type auditRepository struct {
	db *DB
}

func NewAuditRepository(db *DB) *auditRepository {
	return &auditRepository{db: db}
}

// Record implements [audit.Recorder].
func (r *auditRepository) Record(ctx context.Context, e audit.Entry) error {
	prev, err := marshalPayload(e.Prev)
	if err != nil {
		return fmt.Errorf("marshal audit prev: %w", err)
	}
	next, err := marshalPayload(e.Next)
	if err != nil {
		return fmt.Errorf("marshal audit next: %w", err)
	}

	e.FillActorMeta(ctx)
	meta, err := marshalPayload(e.Meta)
	if err != nil {
		return fmt.Errorf("marshal audit meta: %w", err)
	}

	const query = `
		INSERT INTO audit_logs
			(actor_id, action, entity, entity_id, prev, next, meta)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	conn := r.db.GetConn(ctx)
	if _, err := conn.Exec(
		ctx, query,
		e.ActorID, string(e.Action), e.Entity, e.EntityID, prev, next, meta,
	); err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

// marshalPayload returns nil (SQL NULL) for a nil payload, otherwise JSON bytes.
func marshalPayload(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
