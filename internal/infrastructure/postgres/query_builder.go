package postgres

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
)

// QueryBuilder builds a SELECT query.
type QueryBuilder struct {
	table   string
	columns []string
	joins   []string
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

// Join adds an INNER JOIN.
func (q *QueryBuilder) Join(table, on string) *QueryBuilder {
	q.joins = append(q.joins, "JOIN "+table+" ON "+on)
	return q
}

// LeftJoin adds a LEFT JOIN.
func (q *QueryBuilder) LeftJoin(table, on string) *QueryBuilder {
	q.joins = append(q.joins, "LEFT JOIN "+table+" ON "+on)
	return q
}

// RightJoin adds a RIGHT JOIN.
func (q *QueryBuilder) RightJoin(table, on string) *QueryBuilder {
	q.joins = append(q.joins, "RIGHT JOIN "+table+" ON "+on)
	return q
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

// WhereNotNull Implements [repository.Query]
func (q *QueryBuilder) WhereNotNull(column string) {
	q.wheres = append(q.wheres, fmt.Sprintf("%s IS NOT NULL", column))
}

// WhereLike adds a LIKE condition.
// Implements [repository.Query]
func (q *QueryBuilder) WhereLike(column string, value any) {
	if value == nil {
		return
	}

	ph := q.addArg(value)

	q.wheres = append(
		q.wheres,
		fmt.Sprintf("%s LIKE %s", column, ph),
	)
}

// WhereILike adds a PostgreSQL ILIKE condition.
// Implements [repository.Query]
func (q *QueryBuilder) WhereILike(column string, value any) {
	if value == nil {
		return
	}

	ph := q.addArg(value)

	q.wheres = append(
		q.wheres,
		fmt.Sprintf("%s ILIKE %s", column, ph),
	)
}

// WhereBetween adds a half-open range predicate: column >= from AND column < to.
// A zero bound leaves that side open, matching the other Where* helpers, which
// ignore absent input. It always compares the column directly, never a function
// of it, so an index on the column stays usable.
// Implements [repository.Query]
func (q *QueryBuilder) WhereBetween(column string, from, to time.Time) {
	if !from.IsZero() {
		q.wheres = append(
			q.wheres,
			fmt.Sprintf("%s >= %s", column, q.addArg(from)),
		)
	}

	if !to.IsZero() {
		q.wheres = append(
			q.wheres,
			fmt.Sprintf("%s < %s", column, q.addArg(to)),
		)
	}
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
// A non-positive limit disables pagination; callers are expected to clamp
// first (see [repository.Pagination]). The offset saturates instead of being
// allowed to overflow into a negative OFFSET.
func (q *QueryBuilder) Paginate(page, limit int) {
	if limit <= 0 {
		return
	}

	q.limit = limit

	if page > 1 {
		offset := int64(page-1) * int64(limit)
		if offset < 0 || offset > math.MaxInt32 {
			offset = math.MaxInt32
		}

		q.offset = int(offset)
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

	if len(q.joins) > 0 {
		sb.WriteByte(' ')
		sb.WriteString(strings.Join(q.joins, " "))
	}

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

// CountSQL returns the row count for the same FROM/JOIN/WHERE as
// [QueryBuilder.SQL], sharing its arguments and dropping ORDER BY and LIMIT.
//
// A one-to-many JOIN would over-count, so use COUNT(DISTINCT <base>.id) if
// joins are ever added to a counted query.
func (q *QueryBuilder) CountSQL() (string, []any) {
	var b strings.Builder

	b.WriteString("SELECT COUNT(*) FROM ")
	b.WriteString(q.table)

	if len(q.joins) > 0 {
		b.WriteByte(' ')
		b.WriteString(strings.Join(q.joins, " "))
	}

	if len(q.wheres) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(q.wheres, " AND "))
	}

	return b.String(), q.args
}
