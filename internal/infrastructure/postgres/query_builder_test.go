package postgres

import (
	"testing"

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
				qb.WhereIn("gender", []string{"male"})
				qb.WhereNull("deleted_at")
				return qb
			},
			wantSQL: "SELECT id FROM users WHERE role IN ($1, $2) AND gender IN ($3)" +
				" AND deleted_at IS NULL",
			wantArgs: []any{"admin", "user", "male"},
		},
		{
			name: "nil and empty slices add no clause and no argument",
			build: func() *QueryBuilder {
				qb := Select("users", "id")
				qb.WhereIn("role", nil)
				qb.WhereIn("gender", []string{})
				return qb
			},
			wantSQL: "SELECT id FROM users",
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
			// Callers clamp first (repository.Pagination); a non-positive limit
			// must not silently become an unbounded scan.
			name: "non-positive limit omits LIMIT",
			build: func() *QueryBuilder {
				qb := Select("users", "id")
				qb.Paginate(0, 0)
				return qb
			},
			wantSQL: "SELECT id FROM users",
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

// TestQueryBuilder_CountSQL asserts the count matches the page query's FROM and
// WHERE and shares its arguments, dropping ORDER BY and LIMIT. Drift between the
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
