package repository

import "strings"

// Query is the interface a filter writes its clauses into. The postgres
// QueryBuilder is its only implementation.
type Query interface {
	WhereIn(column string, value any)
	WhereNull(column string)

	OrderBy(column, order string)
	Paginate(page, limit int)
}

// Sort represents sorting parameters.
//
// Consumers embed Sort and pass the columns they allow to [Sort.ApplySort], so a
// client-supplied name is never interpolated into the statement.
type Sort struct {
	By    string `query:"sort"`
	Order string `query:"order"`
}

// ApplySort adds an ORDER BY for s.By, but only when it appears in allowed.
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

	// MaxPage caps the page number so the builder's (page-1)*limit offset stays
	// well inside int range; beyond it a query returns no rows anyway.
	MaxPage = 1_000_000
)

// Pagination represents pagination parameters.
type Pagination struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`
}

// ApplyPagination clamps page and limit into their supported ranges, then
// applies them. Without the clamp a request that omits limit would reach the
// builder as zero and produce a statement with no LIMIT at all.
func (p *Pagination) ApplyPagination(q Query) {
	p.normalize()
	q.Paginate(p.Page, p.Limit)
}

// Meta describes pagination for a result set of total rows. Call it after
// ApplyPagination so it reports the clamped page and limit.
func (p Pagination) Meta(total int) PaginationMeta {
	meta := PaginationMeta{
		Page:  p.Page,
		Limit: p.Limit,
		Total: total,
	}

	if p.Limit > 0 {
		meta.TotalPages = (total + p.Limit - 1) / p.Limit
	}

	return meta
}

// normalize clamps page and limit into supported ranges.
func (p *Pagination) normalize() {
	switch {
	case p.Page < 1:
		p.Page = 1
	case p.Page > MaxPage:
		p.Page = MaxPage
	}

	switch {
	case p.Limit < 1:
		p.Limit = DefaultLimit
	case p.Limit > MaxLimit:
		p.Limit = MaxLimit
	}
}
