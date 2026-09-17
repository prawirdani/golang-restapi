package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// recordingQuery captures what the pagination DSL pushes into a query.
type recordingQuery struct {
	page  int
	limit int
}

func (r *recordingQuery) WhereIn(string, any)    {}
func (r *recordingQuery) WhereLike(string, any)  {}
func (r *recordingQuery) WhereILike(string, any) {}
func (r *recordingQuery) WhereNull(string)       {}
func (r *recordingQuery) WhereNotNull(string)    {}
func (r *recordingQuery) OrderBy(string, string) {}

func (r *recordingQuery) Paginate(page, limit int) {
	r.page, r.limit = page, limit
}

func TestPagination_normalize(t *testing.T) {
	tests := []struct {
		name             string
		page, limit      int
		wantPage, wantLi int
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
			p.normalize()

			assert.Equal(t, tt.wantPage, p.Page)
			assert.Equal(t, tt.wantLi, p.Limit)
		})
	}
}

// TestPagination_ApplyPaginationClamps is the regression guard for the bug where
// a request with no query parameters built a query with no LIMIT at all.
func TestPagination_ApplyPaginationClamps(t *testing.T) {
	var q recordingQuery
	p := Pagination{} // exactly what a handler binds from an empty query string

	p.ApplyPagination(&q)

	assert.Equal(t, 1, q.page)
	assert.Equal(t, DefaultLimit, q.limit)

	// The clamped values must also persist on the filter, because the metadata
	// step reads them back after the query has run.
	assert.Equal(t, DefaultLimit, p.Limit)
	assert.Equal(t, 1, p.Page)
}

func TestPagination_SetMeta(t *testing.T) {
	tests := []struct {
		name               string
		page, limit, total int
		want               PaginationMeta
	}{
		{"total pages rounds up", 1, 20, 41, PaginationMeta{Page: 1, Limit: 20, Total: 41, TotalPages: 3}},
		{"total pages divides exactly", 2, 20, 40, PaginationMeta{Page: 2, Limit: 20, Total: 40, TotalPages: 2}},
		{"an empty result reports no pages", 1, 20, 0, PaginationMeta{Page: 1, Limit: 20, Total: 0, TotalPages: 0}},
		{"clamps before computing", 0, 0, 137, PaginationMeta{Page: 1, Limit: DefaultLimit, Total: 137, TotalPages: 7}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Pagination{Page: tt.page, Limit: tt.limit}
			p.SetMeta(tt.total)

			assert.Equal(t, tt.want, p.Meta())
		})
	}
}
