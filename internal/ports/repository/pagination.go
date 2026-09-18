package repository

const (
	// DefaultLimit is applied when a list request omits limit.
	DefaultLimit = 20

	// MaxLimit caps a requested limit so a single client cannot ask for the
	// whole table.
	MaxLimit = 100
)

// Paginator applies pagination to a query.
type Paginator interface {
	ApplyPagination(Query)
}

// PaginationMeta is the pagination part of a cleaned and applied query's.
type PaginationMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// Pagination represents pagination parameters.
type Pagination struct {
	Page  int `query:"page"`
	Limit int `query:"limit"`
}

// ApplyPagination implements [Paginator].
//
// Page and limit are clamped by [Pagination.normalize] before reaching the
// query, so a request that omits them still produces a bounded statement.
func (p *Pagination) ApplyPagination(q Query) {
	if p.Page < 1 {
		p.Page = 1
	}

	switch {
	case p.Limit < 1:
		p.Limit = DefaultLimit
	case p.Limit > MaxLimit:
		p.Limit = MaxLimit
	}
	q.Paginate(p.Page, p.Limit)
}

// PageMeta reports the pagination of a result set of total rows. Call it after
// ApplyPagination so it reports the clamped page and limit.
func (p Pagination) PageMeta(total int) PaginationMeta {
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
