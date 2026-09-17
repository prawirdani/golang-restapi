package repository

import "strings"

// Query is the minimal interface required by query modifiers.
type Query interface {
	WhereIn(column string, value any)
	WhereLike(column string, value any)
	WhereILike(column string, value any)
	WhereNull(column string)
	WhereNotNull(column string)

	OrderBy(column, order string)
	Paginate(page, limit int)
}

// Filterer applies filtering to a query.
type Filterer interface {
	ApplyFilter(Query)
}

// Sorter applies sorting to a query.
type Sorter interface {
	ApplySort(Query)
}

// Paginator applies pagination to a query.
type Paginator interface {
	ApplyPagination(Query)
	Meta() PaginationMeta
	SetMeta(total int)
}

// ApplyQuery applies all capabilities implemented by v.
//
// A value may implement any combination of Filterer, Sorter,
// and Paginator. Unsupported capabilities are simply skipped.
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

// Sort represents sorting parameters.
//
// Sort does not implement Sorter. Consumers should embed Sort
// and implement Sorter themselves to define their allowed sort fields.
//
// Example:
//
//	type UserFilter struct {
//		Sort
//	}
//
//	func (f UserFilter) ApplySort(q Query) {
//		f.Sort.ApplySort(q, map[string]string{
//			"id":         "u.id",
//			"name":       "u.name",
//			"created_at": "u.created_at",
//		})
//	}
//
//	var _ Sorter = UserFilter{}
type Sort struct {
	By    string `query:"sort"`
	Order string `query:"order"`
}

func (s Sort) ApplySort(q Query, allowed map[string]string) {
	column, ok := allowed[s.By]
	if !ok {
		return
	}

	order := strings.ToUpper(s.Order)
	if order != "ASC" && order != "DESC" {
		order = "ASC"
	}

	q.OrderBy(column, order)
}

type PaginationMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

const (
	// DefaultLimit is applied when a list request omits limit.
	DefaultLimit = 20

	// MaxLimit caps a requested limit so a single client cannot ask for the
	// whole table.
	MaxLimit = 100
)

// Pagination represents pagination parameters.
type Pagination struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`
	meta  PaginationMeta
}

// ApplyPagination implements [Paginator].
//
// Page and limit are clamped by [Pagination.normalize] before reaching the
// query, so a request that omits them still produces a bounded statement.
func (p *Pagination) ApplyPagination(q Query) {
	p.normalize()
	q.Paginate(p.Page, p.Limit)
}

// normalize clamps page and limit into supported ranges. Without it a request
// with no limit would build a query with no LIMIT and read the whole table.
func (p *Pagination) normalize() {
	if p.Page < 1 {
		p.Page = 1
	}

	switch {
	case p.Limit < 1:
		p.Limit = DefaultLimit
	case p.Limit > MaxLimit:
		p.Limit = MaxLimit
	}
}

// Meta returns the pagination state recorded by [Pagination.SetMeta].
func (p *Pagination) Meta() PaginationMeta {
	return p.meta
}

// SetMeta records the total row count alongside the clamped page and limit.
func (p *Pagination) SetMeta(total int) {
	p.normalize()

	totalPages := 0
	if p.Limit > 0 {
		totalPages = (total + p.Limit - 1) / p.Limit
	}

	p.meta = PaginationMeta{
		Page:       p.Page,
		Limit:      p.Limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
