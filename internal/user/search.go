package user

import (
	"github.com/prawirdani/golang-restapi/internal/ports/repository"
	"github.com/prawirdani/golang-restapi/internal/rbac"
)

type Filter struct {
	Gender []string `query:"gender" json:"gender,omitempty"`
	Role   []string `query:"role"   json:"role,omitempty"`
}

type Search struct {
	Filter
	repository.Sorting
	repository.Pagination
	meta repository.QueryMeta[Filter]
}

var userSortFields = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

// ApplySort implements [repository.Sorter]
//
// The pointer receiver lets the sanitised sort reach the metadata, which echoes
// these fields.
func (s *Search) ApplySort(q repository.Query) {
	s.Sorting.ApplySort(q, userSortFields)
}

// ApplyFilter implements [repository.Filterer].
//
// Sanitisation happens here: values the domain does not recognise are dropped
// and the rest are canonicalised, so the query filters on exactly the values
// [Search.SetMeta] reports as applied. The slices stay nil when nothing
// survives, which leaves the filter at its zero value so the metadata omits it.
func (s *Search) ApplyFilter(q repository.Query) {
	var validGenders []string

	for _, v := range s.Gender {
		if g, ok := ParseGender(v); ok {
			validGenders = append(validGenders, string(g))
		}
	}

	var validRoles []string

	for _, v := range s.Role {
		if r, ok := rbac.ParseRole(v); ok {
			validRoles = append(validRoles, string(r))
		}
	}

	s.Gender = validGenders
	s.Role = validRoles

	q.WhereIn("gender", s.Gender)
	q.WhereIn("role", s.Role)
}

func (s *Search) SetMeta(total int) {
	s.meta = repository.QueryMeta[Filter]{
		Filter:     s.Filter, // applied only sanitized by ApplyFilter
		Sort:       s.Sorting,
		Pagination: s.PageMeta(total),
	}
}

func (s Search) Meta() repository.QueryMeta[Filter] {
	return s.meta
}
