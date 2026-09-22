package repository

import "time"

// Query is the minimal interface required by query modifiers.
type Query interface {
	WhereIn(column string, value any)
	WhereLike(column string, value any)
	WhereILike(column string, value any)
	WhereNull(column string)
	WhereNotNull(column string)
	WhereBetween(column string, from, to time.Time)

	OrderBy(column, order string)
	Paginate(page, limit int)
}

// Filterer applies filtering to a query.
//
// Implementations may sanitise here. This is the place to drop or canonicalise
// values the domain does not recognise, so that a query filters on exactly the
// values the response reports as applied.
type Filterer interface {
	ApplyFilter(Query)
}

// ApplyQuery applies all capabilities implemented by v.
//
// A value may implement any combination of Filterer, Sorter, and Paginator.
// Unsupported capabilities are simply skipped.
func ApplyQuery(q Query, v any) {
	if f, ok := v.(Filterer); ok {
		f.ApplyFilter(q)
	}

	if s, ok := v.(Sorter); ok {
		s.ApplySort(q)
	}

	if p, ok := v.(Paginator); ok {
		p.ApplyPagination(q)
	}
}

type QueryMeta[TFilter any] struct {
	Filter     TFilter        `json:"filter,omitempty,omitzero"`
	Sort       Sorting        `json:"sort,omitzero"`
	Pagination PaginationMeta `json:"pagination,omitzero"`
}
