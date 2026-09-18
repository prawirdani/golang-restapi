package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// recordingQuery captures what the query DSL pushes into a query.
type recordingQuery struct {
	page    int
	limit   int
	orderBy string
}

func (r *recordingQuery) WhereIn(string, any)    {}
func (r *recordingQuery) WhereLike(string, any)  {}
func (r *recordingQuery) WhereILike(string, any) {}
func (r *recordingQuery) WhereNull(string)       {}
func (r *recordingQuery) WhereNotNull(string)    {}

func (r *recordingQuery) OrderBy(column, order string) {
	r.orderBy = column + " " + order
}

func (r *recordingQuery) Paginate(page, limit int) {
	r.page, r.limit = page, limit
}

// TestPagination_ApplyPaginationClamps is the regression guard for the bug where
// a request with no query parameters built a query with no LIMIT at all. It also
// asserts the clamped values persist on the filter, because the metadata step
// reads them back after the query has run.
func TestPagination_ApplyPaginationClamps(t *testing.T) {
	tests := []struct {
		name                string
		page, limit         int
		wantPage, wantLimit int
	}{
		{"an unset request gets defaults", 0, 0, 1, DefaultLimit},
		{"negative values get defaults too", -5, -1, 1, DefaultLimit},
		{"a missing page becomes the first page", 0, 20, 1, 20},
		{"a limit above the cap is clamped", 1, MaxLimit + 1, 1, MaxLimit},
		{"valid values are left alone", 3, 50, 3, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Pagination{Page: tt.page, Limit: tt.limit}

			var q recordingQuery
			p.ApplyPagination(&q)

			assert.Equal(t, tt.wantPage, q.page)
			assert.Equal(t, tt.wantLimit, q.limit)

			assert.Equal(t, tt.wantPage, p.Page)
			assert.Equal(t, tt.wantLimit, p.Limit)
		})
	}
}

func TestPagination_PageMeta(t *testing.T) {
	tests := []struct {
		name               string
		page, limit, total int
		want               PaginationMeta
	}{
		{"total pages rounds up", 1, 20, 41, PaginationMeta{Page: 1, Limit: 20, Total: 41, TotalPages: 3}},
		{"total pages divide exactly", 2, 20, 40, PaginationMeta{Page: 2, Limit: 20, Total: 40, TotalPages: 2}},
		{"an empty result reports no pages", 1, 20, 0, PaginationMeta{Page: 1, Limit: 20, Total: 0, TotalPages: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Pagination{Page: tt.page, Limit: tt.limit}

			assert.Equal(t, tt.want, p.PageMeta(tt.total))
		})
	}
}

func TestSorting_ApplySort(t *testing.T) {
	allowed := map[string]string{"id": "u.id", "created_at": "u.created_at"}

	tests := []struct {
		name      string
		by, order string
		want      string
	}{
		{name: "allow-listed column", by: "created_at", order: "desc", want: "u.created_at DESC"},
		{name: "order is upper-cased", by: "id", order: "asc", want: "u.id ASC"},
		{name: "order defaults to ascending", by: "id", want: "u.id ASC"},
		{name: "unknown column is ignored", by: "password", order: "desc"},
		{name: "empty column is ignored", order: "desc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var q recordingQuery

			s := Sorting{By: tt.by, Order: tt.order}
			s.ApplySort(&q, allowed)

			assert.Equal(t, tt.want, q.orderBy)
		})
	}
}

// TestSorting_ApplySortSanitisesRequest asserts the request fields are left
// describing what was applied, since the metadata echoes them: an unlisted
// column is cleared and a missing direction becomes ASC.
func TestSorting_ApplySortSanitisesRequest(t *testing.T) {
	allowed := map[string]string{"id": "u.id"}

	tests := []struct {
		name              string
		by, order         string
		wantBy, wantOrder string
	}{
		{name: "applied sort is canonicalised", by: "id", order: "desc", wantBy: "id", wantOrder: "DESC"},
		{name: "missing direction defaults to ascending", by: "id", wantBy: "id", wantOrder: "ASC"},
		{name: "unlisted column is cleared", by: "password", order: "desc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Sorting{By: tt.by, Order: tt.order}

			var q recordingQuery
			s.ApplySort(&q, allowed)

			assert.Equal(t, tt.wantBy, s.By)
			assert.Equal(t, tt.wantOrder, s.Order)
		})
	}
}
