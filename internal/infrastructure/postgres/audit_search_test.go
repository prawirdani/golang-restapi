package postgres

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/prawirdani/golang-restapi/internal/ports/repository"
	"github.com/stretchr/testify/assert"
)

// TestAuditSearch_DateRange asserts the audit filter wires the half-open date
// predicate through the shared query builder: a range binds both bounds in
// order, and no range adds no created_at predicate at all.
func TestAuditSearch_DateRange(t *testing.T) {
	build := func(search *audit.Search) (string, []any) {
		qb := Select("audit_logs AS e", "e.id").
			LeftJoin("users AS u", "e.actor_id=u.id")
		repository.ApplyQuery(qb, search)
		return qb.SQL()
	}

	t.Run("a date range binds both bounds", func(t *testing.T) {
		from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

		search := &audit.Search{
			Filter: audit.Filter{
				DateRange: repository.DateRange{From: "2024-01-01", To: "2024-01-31"},
			},
		}

		sql, args := build(search)

		assert.Contains(t, sql, "e.created_at >= $1 AND e.created_at < $2")
		assert.Equal(t, []any{from, to}, args)
	})

	t.Run("an empty range adds no predicate", func(t *testing.T) {
		sql, args := build(&audit.Search{})

		assert.NotContains(t, sql, "e.created_at")
		assert.Empty(t, args)
	})
}

// TestAuditSearch_Actor asserts a single free-text term searches the actor: a
// full UUID matches actor_id exactly, anything else matches the name with ILIKE,
// and LIKE wildcards in the term are escaped rather than interpreted.
func TestAuditSearch_Actor(t *testing.T) {
	build := func(search *audit.Search) (string, []any) {
		qb := Select("audit_logs AS e", "e.id").
			LeftJoin("users AS u", "e.actor_id=u.id")
		repository.ApplyQuery(qb, search)
		return qb.SQL()
	}

	t.Run("a full UUID matches actor_id exactly", func(t *testing.T) {
		id := uuid.MustParse("0193a5b0-0000-7000-8000-000000000000")
		search := &audit.Search{Filter: audit.Filter{Actor: id.String()}}

		sql, args := build(search)

		assert.Contains(t, sql, "e.actor_id IN ($1)")
		assert.Equal(t, []any{id}, args)
	})

	t.Run("anything else matches the actor name", func(t *testing.T) {
		search := &audit.Search{Filter: audit.Filter{Actor: "jane"}}

		sql, args := build(search)

		assert.Contains(t, sql, "u.name ILIKE $1")
		assert.Equal(t, []any{"%jane%"}, args)
	})

	t.Run("the term is trimmed before use", func(t *testing.T) {
		search := &audit.Search{Filter: audit.Filter{Actor: "  jane  "}}

		_, args := build(search)

		assert.Equal(t, []any{"%jane%"}, args)
		assert.Equal(t, "jane", search.Actor)
	})

	t.Run("LIKE wildcards in the term are escaped", func(t *testing.T) {
		search := &audit.Search{Filter: audit.Filter{Actor: `a_b%c`}}

		_, args := build(search)

		assert.Equal(t, []any{`%a\_b\%c%`}, args)
	})

	t.Run("a blank term adds no predicate", func(t *testing.T) {
		search := &audit.Search{Filter: audit.Filter{Actor: "   "}}

		sql, args := build(search)

		assert.NotContains(t, sql, "e.actor_id IN")
		assert.NotContains(t, sql, "ILIKE")
		assert.Empty(t, args)
		assert.Equal(t, "", search.Actor)
	})
}

// TestAuditSearch_Entity asserts only entity names the domain records reach the
// query: unknown ones are dropped from both the predicate and the echoed filter,
// and a request with nothing valid left adds no predicate at all.
func TestAuditSearch_Entity(t *testing.T) {
	build := func(search *audit.Search) (string, []any) {
		qb := Select("audit_logs AS e", "e.id")
		repository.ApplyQuery(qb, search)
		return qb.SQL()
	}

	t.Run("valid names are kept and canonicalised", func(t *testing.T) {
		search := &audit.Search{Filter: audit.Filter{Entity: []string{" USER ", "session"}}}

		sql, args := build(search)

		assert.Contains(t, sql, "e.entity IN ($1, $2)")
		assert.Equal(t, []any{"user", "session"}, args)
		assert.Equal(t, []string{"user", "session"}, search.Entity)
	})

	t.Run("unknown names are dropped", func(t *testing.T) {
		search := &audit.Search{Filter: audit.Filter{Entity: []string{"auth", "user", "users"}}}

		sql, args := build(search)

		assert.Contains(t, sql, "e.entity IN ($1)")
		assert.Equal(t, []any{"user"}, args)
		assert.Equal(t, []string{"user"}, search.Entity)
	})

	t.Run("only unknown names add no predicate", func(t *testing.T) {
		search := &audit.Search{Filter: audit.Filter{Entity: []string{"auth", "nope"}}}

		sql, args := build(search)

		assert.NotContains(t, sql, "e.entity")
		assert.Empty(t, args)
		assert.Nil(t, search.Entity)
	})
}
