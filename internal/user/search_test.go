package user

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/prawirdani/golang-restapi/internal/ports/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchRecorder captures what a Search pushes into a query.
type searchRecorder struct {
	whereIn map[string][]string
	orderBy string
	page    int
	limit   int
}

func (r *searchRecorder) WhereIn(column string, value any) {
	if r.whereIn == nil {
		r.whereIn = make(map[string][]string)
	}

	values, _ := value.([]string)
	r.whereIn[column] = values
}

func (r *searchRecorder) WhereLike(string, any)  {}
func (r *searchRecorder) WhereILike(string, any) {}
func (r *searchRecorder) WhereNull(string)       {}
func (r *searchRecorder) WhereNotNull(string)    {}

func (r *searchRecorder) WhereBetween(string, time.Time, time.Time) {}

func (r *searchRecorder) OrderBy(column, order string) {
	r.orderBy = column + " " + order
}

func (r *searchRecorder) Paginate(page, limit int) {
	r.page, r.limit = page, limit
}

// TestSearch_ApplyFilterSanitisesValues: unrecognised values are dropped and the
// rest canonicalised, in place and in the query, so neither the statement nor
// the reported metadata carries a value the domain does not know.
func TestSearch_ApplyFilterSanitisesValues(t *testing.T) {
	tests := []struct {
		name                 string
		gender, role         []string
		wantGender, wantRole []string
	}{
		{
			// Nothing survives sanitisation, so the applied sets stay nil and
			// the metadata omits the filter rather than emitting {}.
			name:   "unrecognised values are dropped",
			gender: []string{"banana"},
			role:   []string{"root"},
		},
		{
			name:       "casing is canonicalised",
			gender:     []string{"m", " f "},
			role:       []string{"ADMIN"},
			wantGender: []string{"M", "F"},
			wantRole:   []string{"admin"},
		},
		{
			name:       "recognised values keep the client's order",
			gender:     []string{"o", "m"},
			role:       []string{"user", "system"},
			wantGender: []string{"O", "M"},
			wantRole:   []string{"user", "system"},
		},
		{
			name:       "unknown values are dropped alongside known ones",
			gender:     []string{"M", "x"},
			role:       []string{"admin", "nope"},
			wantGender: []string{"M"},
			wantRole:   []string{"admin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Search{Filter: Filter{Gender: tt.gender, Role: tt.role}}

			var q searchRecorder
			s.ApplyFilter(&q)

			assert.Equal(t, tt.wantGender, s.Gender)
			assert.Equal(t, tt.wantRole, s.Role)
			assert.Equal(t, tt.wantGender, q.whereIn["gender"])
			assert.Equal(t, tt.wantRole, q.whereIn["role"])
		})
	}
}

func TestSearch_ApplySortUsesAllowList(t *testing.T) {
	tests := []struct {
		name      string
		by, order string
		want      string
	}{
		{name: "allow-listed column", by: "created_at", order: "desc", want: "created_at DESC"},
		{name: "order defaults to ascending", by: "id", want: "id ASC"},
		{name: "unlisted column is ignored", by: "password", order: "desc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Search{Sorting: repository.Sorting{By: tt.by, Order: tt.order}}

			var q searchRecorder
			s.ApplySort(&q)

			assert.Equal(t, tt.want, q.orderBy)
		})
	}
}

// TestSearch_SetMeta asserts the metadata describes the applied query: the
// sanitised filter and the clamped pagination.
func TestSearch_SetMeta(t *testing.T) {
	s := &Search{
		Filter:     Filter{Gender: []string{"m", "banana"}, Role: []string{"ADMIN"}},
		Sorting:    repository.Sorting{By: "created_at", Order: "desc"},
		Pagination: repository.Pagination{},
	}

	var q searchRecorder
	repository.ApplyQuery(&q, s)
	s.SetMeta(41)

	got := s.Meta()

	assert.Equal(t, Filter{Gender: []string{"M"}, Role: []string{"admin"}}, got.Filter)
	assert.Equal(t, repository.Sorting{By: "created_at", Order: "DESC"}, got.Sort)
	assert.Equal(t, repository.PaginationMeta{Page: 1, Limit: repository.DefaultLimit, Total: 41, TotalPages: 3}, got.Pagination)
}

// TestSearch_MetaOmitsEmptyFilter asserts an unrecognised filter leaves the
// metadata's filter at its zero value, so the response drops the key instead of
// serialising an empty object.
func TestSearch_MetaOmitsEmptyFilter(t *testing.T) {
	s := &Search{Filter: Filter{Gender: []string{"banana"}, Role: []string{"root"}}}

	var q searchRecorder
	repository.ApplyQuery(&q, s)
	s.SetMeta(0)

	got := s.Meta()

	assert.Nil(t, got.Filter.Gender)
	assert.Nil(t, got.Filter.Role)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "filter")
	assert.Contains(t, string(raw), `"pagination"`)
}

// TestSearch_ApplySortSanitisesRequest asserts the sort the metadata echoes is
// the one that was applied, not the one that was asked for.
func TestSearch_ApplySortSanitisesRequest(t *testing.T) {
	tests := []struct {
		name              string
		by, order         string
		wantBy, wantOrder string
	}{
		{name: "applied sort is canonicalised", by: "id", order: "desc", wantBy: "id", wantOrder: "DESC"},
		{name: "missing direction defaults to ascending", by: "created_at", wantBy: "created_at", wantOrder: "ASC"},
		{name: "unlisted column is cleared", by: "password", order: "desc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Search{Sorting: repository.Sorting{By: tt.by, Order: tt.order}}

			var q searchRecorder
			s.ApplySort(&q)

			assert.Equal(t, tt.wantBy, s.By)
			assert.Equal(t, tt.wantOrder, s.Order)
		})
	}
}
