package repository

import "strings"

// Sorter applies sorting to a query.
type Sorter interface {
	ApplySort(Query)
}

// Sorting represents sorting parameters.
//
// Sorting does not implement Sorter. Consumers should embed Sorting
// and implement Sorter themselves to define their allowed sort fields.
//
// Example:
//
//	type UserFilter struct {
//		Sorting
//	}
//
//	func (f UserFilter) ApplySort(q Query) {
//		f.Sorting.ApplySort(q, map[string]string{
//			"id":         "u.id",
//			"name":       "u.name",
//			"created_at": "u.created_at",
//		})
//	}
//
//	var _ Sorter = UserFilter{}
type Sorting struct {
	By    string `query:"sort"  json:"by,omitempty"`
	Order string `query:"order" json:"order,omitempty"`
}

// ApplySort adds an ORDER BY for the requested column, but only when the
// allow-list recognises it. It leaves the request fields describing what was
// applied — cleared, or with a canonical direction — because the metadata echoes
// them. That needs a pointer receiver: on a copy the assignments are lost.
func (s *Sorting) ApplySort(q Query, allowed map[string]string) {
	column, ok := allowed[s.By]
	if !ok {
		s.By = ""
		s.Order = ""
		return
	}

	order := strings.ToUpper(s.Order)
	if order != "ASC" && order != "DESC" {
		order = "ASC"
	}

	s.Order = order

	q.OrderBy(column, order)
}
