package postgres

import (
	"fmt"
	"reflect"
	"strings"
)

// QueryBuilder builds a SELECT query.
type QueryBuilder struct {
	table   string
	columns []string
	wheres  []string
	args    []any
	orderBy string
	limit   int
	offset  int
}

// Select creates a new SELECT query.
func Select(table string, columns ...string) *QueryBuilder {
	return &QueryBuilder{
		table:   table,
		columns: columns,
	}
}

// WhereIn adds an IN condition.
//
// Nil and empty slices are ignored.
// Implements [repository.Query]
func (q *QueryBuilder) WhereIn(column string, value any) {
	if value == nil {
		return
	}

	v := reflect.ValueOf(value)

	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return
	}

	if v.Len() == 0 {
		return
	}

	placeholders := make([]string, v.Len())

	for i := 0; i < v.Len(); i++ {
		placeholders[i] = q.addArg(v.Index(i).Interface())
	}

	q.wheres = append(
		q.wheres,
		fmt.Sprintf(
			"%s IN (%s)",
			column,
			strings.Join(placeholders, ", "),
		),
	)
}

// WhereNull Implements [repository.Query]
func (q *QueryBuilder) WhereNull(column string) {
	q.wheres = append(q.wheres, fmt.Sprintf("%s IS NULL", column))
}

// OrderBy adds an ORDER BY clause.
// Implements [repository.Query]
func (q *QueryBuilder) OrderBy(column, order string) {
	if column == "" {
		return
	}

	q.orderBy = column

	if order != "" {
		q.orderBy += " " + order
	}
}

// Paginate applies offset-based pagination.
// Implements [repository.Query]
//
// A non-positive limit disables pagination. Callers clamp page and limit first
// (see [repository.Pagination]), which keeps (page-1)*limit inside int range.
func (q *QueryBuilder) Paginate(page, limit int) {
	if limit <= 0 {
		return
	}

	q.limit = limit

	if page > 1 {
		q.offset = (page - 1) * limit
	}
}

func (q *QueryBuilder) addArg(value any) string {
	q.args = append(q.args, value)

	return fmt.Sprintf("$%d", len(q.args))
}

// SQL returns the generated SQL and its arguments.
func (q *QueryBuilder) SQL() (string, []any) {
	var sb strings.Builder

	sb.WriteString("SELECT ")
	sb.WriteString(strings.Join(q.columns, ", "))

	sb.WriteString(" FROM ")
	sb.WriteString(q.table)

	if len(q.wheres) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(q.wheres, " AND "))
	}

	if q.orderBy != "" {
		sb.WriteString(" ORDER BY ")
		sb.WriteString(q.orderBy)
	}

	if q.limit > 0 {
		fmt.Fprintf(
			&sb,
			" LIMIT %d OFFSET %d",
			q.limit,
			q.offset,
		)
	}

	return sb.String(), q.args
}

// CountSQL returns the row count for the same FROM/WHERE as [QueryBuilder.SQL],
// sharing its arguments and dropping ORDER BY and LIMIT.
func (q *QueryBuilder) CountSQL() (string, []any) {
	var b strings.Builder

	b.WriteString("SELECT COUNT(*) FROM ")
	b.WriteString(q.table)

	if len(q.wheres) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(q.wheres, " AND "))
	}

	return b.String(), q.args
}
