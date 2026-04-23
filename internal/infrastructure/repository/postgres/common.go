// Package postgres provides PostgreSQL implementations of the repository
// interfaces defined in the domain layer. It handles database connections and query execution
package postgres

import (
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func uniqueViolationErr(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraintName {
		return true
	}

	return false
}

func noRowsErr(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// generateInsertQuery creates a complete INSERT statement from [pgx.NamedArgs]
// Example: GenerateInsertQuery("users", pgx.NamedArgs{"name": "John", "age": 30})
// Returns: "INSERT INTO users (age, name) VALUES (@age, @name)"
func generateInsertQuery(table string, args pgx.NamedArgs) string {
	if len(args) == 0 {
		return "INSERT INTO " + table + " DEFAULT VALUES"
	}

	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("INSERT INTO ")
	b.WriteString(table)
	b.WriteString(" (")

	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
	}
	b.WriteString(") VALUES (")

	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("@")
		b.WriteString(k)
	}
	b.WriteString(")")

	return b.String()
}

// generateUpdateQuery creates a base UPDATE statement from [pgx.NamedArgs].
//
// It generates the SET clause for all arguments, EXCLUDING those specified in
// `whereColumns`. It then appends the WHERE clause using the columns
// provided in `whereColumns` (e.g., WHERE col1=@col1 AND col2=@col2).
//
// Example 1: generateUpdateQuery("users", pgx.NamedArgs{"name": "John", "age": 30, "id": 1}, "id")
// Returns: "UPDATE users SET age=@age, name=@name WHERE id=@id"
//
// Example 2 (No WHERE clause): generateUpdateQuery("config", pgx.NamedArgs{"value": 1})
// Returns: "UPDATE config SET value=@value"
func generateUpdateQuery(table string, args pgx.NamedArgs, whereColumns ...string) string {
	if len(args) == 0 {
		return ""
	}

	// Map of WHERE columns for quick lookup
	whereMap := make(map[string]struct{}, len(whereColumns))
	for _, col := range whereColumns {
		whereMap[col] = struct{}{}
	}

	var b strings.Builder
	b.WriteString("UPDATE ")
	b.WriteString(table)
	b.WriteString(" SET ")

	setCount := 0
	for k := range args {
		if _, isWhere := whereMap[k]; isWhere {
			continue
		}
		if setCount > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString("=@")
		b.WriteString(k)
		setCount++
	}

	if setCount == 0 {
		return ""
	}

	if len(whereColumns) > 0 {
		b.WriteString(" WHERE ")
		for i, col := range whereColumns {
			if i > 0 {
				b.WriteString(" AND ")
			}
			b.WriteString(col)
			b.WriteString("=@")
			b.WriteString(col)
		}
	}

	return b.String()
}
