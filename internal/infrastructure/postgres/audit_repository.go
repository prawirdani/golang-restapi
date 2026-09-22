package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/prawirdani/golang-restapi/internal/ports/repository"
)

type auditRepository struct {
	db *DB
}

func NewAuditRepository(db *DB) *auditRepository {
	return &auditRepository{db: db}
}

// List implements [audit.Reader].
func (r *auditRepository) List(ctx context.Context, search *audit.Search) ([]audit.EntryWithActor, error) {
	if search == nil {
		return nil, errors.New("search is nil")
	}

	conn := r.db.GetConn(ctx)

	qb := Select("audit_logs AS e",
		"e.id", "e.action", "e.entity", "e.entity_id", "e.prev", "e.next", "e.meta", "e.created_at",
		"e.actor_id", "u.name AS actor_name").LeftJoin("users AS u", "e.actor_id=u.id")
	repository.ApplyQuery(qb, search)

	query, args := qb.SQL()

	rows := make([]auditRow, 0)
	if err := pgxscan.Select(ctx, conn, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}

	fmt.Println(query, args)

	entries := make([]audit.EntryWithActor, 0, len(rows))
	for i := range rows {
		entries = append(entries, rows[i].toEntry())
	}

	cQuery, cArgs := qb.CountSQL()
	var totalData int
	if err := pgxscan.Get(ctx, conn, &totalData, cQuery, cArgs...); err != nil {
		return nil, fmt.Errorf("count list audit: %w", err)
	}
	search.SetMeta(totalData)

	return entries, nil
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

// auditRow is one joined audit row. The actor columns are pointers so a LEFT
// JOIN miss scans as nil instead of failing on a NULL: a system-written entry
// has no actor_id, and its user row may have been removed. The domain type keeps
// plain values, so the mapping happens here, at the persistence boundary.
type auditRow struct {
	ID        int            `db:"id"`
	Action    audit.Action   `db:"action"`
	Entity    string         `db:"entity"`
	EntityID  string         `db:"entity_id"`
	Prev      any            `db:"prev"`
	Next      any            `db:"next"`
	Meta      map[string]any `db:"meta"`
	CreatedAt time.Time      `db:"created_at"`
	ActorID   *uuid.UUID     `db:"actor_id"`
	ActorName *string        `db:"actor_name"`
}

// toEntry maps a row to the domain view, leaving Actor nil for an entry that
// has no actor.
func (r auditRow) toEntry() audit.EntryWithActor {
	entry := audit.EntryWithActor{
		ID:        r.ID,
		Action:    r.Action,
		Entity:    r.Entity,
		EntityID:  r.EntityID,
		Prev:      r.Prev,
		Next:      r.Next,
		Meta:      r.Meta,
		CreatedAt: r.CreatedAt,
	}

	if r.ActorID != nil {
		actor := &audit.Actor{ID: *r.ActorID}
		if r.ActorName != nil {
			actor.Name = *r.ActorName
		}

		entry.Actor = actor
	}

	return entry
}
