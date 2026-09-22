package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestQueryBuilder_SQL pins the generated statement and argument order, which is
// what the database sees. It covers the branches that produced real bugs:
// pagination bounds and argument numbering.
func TestQueryBuilder_SQL(t *testing.T) {
	tests := []struct {
		name     string
		build    func() *QueryBuilder
		wantSQL  string
		wantArgs []any
	}{
		{
			name: "filters only",
			build: func() *QueryBuilder {
				qb := Select("users", "id", "name")
				qb.WhereNull("deleted_at")
				return qb
			},
			wantSQL: "SELECT id, name FROM users WHERE deleted_at IS NULL",
		},
		{
			name: "arguments are numbered in order of use",
			build: func() *QueryBuilder {
				qb := Select("users", "id")
				qb.WhereIn("role", []string{"admin", "user"})
				qb.WhereLike("name", "jo%")
				qb.WhereILike("email", "%EXAMPLE%")
				qb.WhereNotNull("email_verified_at")
				return qb
			},
			wantSQL: "SELECT id FROM users WHERE role IN ($1, $2) AND name LIKE $3" +
				" AND email ILIKE $4 AND email_verified_at IS NOT NULL",
			wantArgs: []any{"admin", "user", "jo%", "%EXAMPLE%"},
		},
		{
			name: "nil and empty values add no clause and no argument",
			build: func() *QueryBuilder {
				qb := Select("users", "id")
				qb.WhereIn("role", nil)
				qb.WhereIn("gender", []string{})
				qb.WhereLike("name", nil)
				return qb
			},
			wantSQL: "SELECT id FROM users",
		},
		{
			name: "joins come before the where clause",
			build: func() *QueryBuilder {
				qb := Select("users", "u.id")
				qb.Join("roles r", "r.id = u.role_id")
				qb.LeftJoin("teams t", "t.id = u.team_id")
				qb.WhereNull("u.deleted_at")
				qb.OrderBy("u.id", "DESC")
				return qb
			},
			wantSQL: "SELECT u.id FROM users JOIN roles r ON r.id = u.role_id" +
				" LEFT JOIN teams t ON t.id = u.team_id WHERE u.deleted_at IS NULL ORDER BY u.id DESC",
		},
		{
			name: "empty order column adds nothing",
			build: func() *QueryBuilder {
				qb := Select("users", "id")
				qb.OrderBy("", "DESC")
				return qb
			},
			wantSQL: "SELECT id FROM users",
		},
		{
			name: "first page",
			build: func() *QueryBuilder {
				qb := Select("users", "id")
				qb.Paginate(1, 20)
				return qb
			},
			wantSQL: "SELECT id FROM users LIMIT 20 OFFSET 0",
		},
		{
			name: "offset is (page-1)*limit",
			build: func() *QueryBuilder {
				qb := Select("users", "id")
				qb.Paginate(3, 20)
				return qb
			},
			wantSQL: "SELECT id FROM users LIMIT 20 OFFSET 40",
		},
		{
			// Callers clamp first (repository.Pagination), but a non-positive
			// limit must not be turned into an unbounded scan by accident.
			name: "non-positive limit omits LIMIT",
			build: func() *QueryBuilder {
				qb := Select("users", "id")
				qb.Paginate(0, 0)
				return qb
			},
			wantSQL: "SELECT id FROM users",
		},
		{
			// (page-1)*limit overflows int64 for an absurd page; the offset must
			// saturate rather than reach Postgres as a negative OFFSET.
			name: "huge page saturates the offset",
			build: func() *QueryBuilder {
				qb := Select("users", "id")
				qb.Paginate(1<<62, 100)
				return qb
			},
			wantSQL: "SELECT id FROM users LIMIT 100 OFFSET 2147483647",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args := tt.build().SQL()

			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestQueryBuilder_WhereBetween pins the range predicate: a half-open pair of
// bounds, each side independently open when its bound is the zero time.
func TestQueryBuilder_WhereBetween(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		from, to time.Time
		wantSQL  string
		wantArgs []any
	}{
		{
			name:     "both bounds are half-open",
			from:     from,
			to:       to,
			wantSQL:  "SELECT id FROM audit_logs WHERE created_at >= $1 AND created_at < $2",
			wantArgs: []any{from, to},
		},
		{
			name:     "from only leaves the upper side open",
			from:     from,
			wantSQL:  "SELECT id FROM audit_logs WHERE created_at >= $1",
			wantArgs: []any{from},
		},
		{
			name:     "to only leaves the lower side open",
			to:       to,
			wantSQL:  "SELECT id FROM audit_logs WHERE created_at < $1",
			wantArgs: []any{to},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := Select("audit_logs", "id")
			qb.WhereBetween("created_at", tt.from, tt.to)

			sql, args := qb.SQL()

			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}

// TestQueryBuilder_CountSQL asserts the count matches the page query's
// FROM/JOIN/WHERE and arguments, and drops ORDER BY and LIMIT. Drift between the
// two statements would make total/total_pages disagree with the returned rows.
func TestQueryBuilder_CountSQL(t *testing.T) {
	qb := Select("users", "id")
	qb.WhereNull("deleted_at")
	qb.WhereIn("role", []string{"admin", "user"})
	qb.OrderBy("id", "DESC")
	qb.Paginate(2, 20)

	sql, args := qb.CountSQL()

	assert.Equal(t, "SELECT COUNT(*) FROM users WHERE deleted_at IS NULL AND role IN ($1, $2)", sql)
	assert.Equal(t, []any{"admin", "user"}, args)
}
