package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// recordingQuery captures what a filter pushes into a query.
//
// It exists because this package cannot import the postgres builder — postgres
// imports repository, so the dependency would be a cycle — and normalize is
// unexported, so an external test package cannot reach it either.
type recordingQuery struct {
	page  int
	limit int
}

func (r *recordingQuery) WhereIn(string, any)    {}
func (r *recordingQuery) WhereNull(string)       {}
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
		{"a page above the cap is clamped", MaxPage + 1, 20, MaxPage, 20},
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

// TestPagination_ApplyPaginationClamps guards the bug where a request with no
// query parameters built a statement with no LIMIT at all, and asserts the
// clamped values persist on the filter for the metadata step.
func TestPagination_ApplyPaginationClamps(t *testing.T) {
	var q recordingQuery
	p := Pagination{} // what a handler binds from an empty query string

	p.ApplyPagination(&q)

	assert.Equal(t, 1, q.page)
	assert.Equal(t, DefaultLimit, q.limit)

	assert.Equal(t, 1, p.Page)
	assert.Equal(t, DefaultLimit, p.Limit)
}

// TestPagination_Meta covers the metadata a list response reports. It reads the
// clamped page and limit, so it is called after ApplyPagination.
func TestPagination_Meta(t *testing.T) {
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

			assert.Equal(t, tt.want, p.Meta(tt.total))
		})
	}
}
