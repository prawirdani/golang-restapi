---
name: gorest-repositories
description: "PostgreSQL repository conventions for github.com/prawirdani/golang-restapi — pgx.NamedArgs + generateInsertQuery/generateUpdateQuery builders, pgxscan scanning, transaction-aware connections (db.GetConn/IsTxConn + FOR UPDATE), error translation with uniqueViolationErr/noRowsErr, and method-level docs. Use when writing or reviewing any postgres repository in internal/infrastructure/postgres/."
user-invocable: true
license: MIT
compatibility: Designed for AI coding agents working in the golang-restapi repository.
metadata:
  author: prawirdani
  version: "1.0.0"
  module: github.com/prawirdani/golang-restapi
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Agent
---

**Persona:** You are the data-access reviewer. Repositories are dumb, deterministic SQL layer: no business rules, no multi-step orchestration (that is the service's job via `Transact`), no raw `database/sql` — pgx + pgxscan only.

## Rules

1. **Struct shape is fixed:** unexported `userRepository struct { db *DB }` + `NewUserRepository(db *DB) *userRepository` returning the unexported type. Every method documents the interface it implements: `// Store implements [user.Repository].` (bracketed reference, including multi-interface: `// GetByEmail implements [user.Repository] [auth.UserRepository].`).

2. **Nil guards first:** `if u == nil { return errors.New("user is nil") }` at the top of every mutator.

3. **Queries use `pgx.NamedArgs` with snake_case keys** and the shared builders from `common.go`:
   ```go
   args := pgx.NamedArgs{
       "id":       u.ID,
       "name":     u.Name,
       "email":    u.Email,
       ...
   }
   query := generateInsertQuery("users", args) + "\nRETURNING created_at, updated_at"
   conn := r.db.GetConn(ctx)
   if err := conn.QueryRow(ctx, query, args).Scan(&u.CreatedAt, &u.UpdatedAt); err != nil { ... }
   ```
   - Insert: `generateInsertQuery(table, args)` — columns sorted alphabetically by the builder; append `RETURNING` for server-generated timestamps.
   - Update: `generateUpdateQuery(table, args, "id")` — pass WHERE columns last; add `"updated_at": "NOW()"` to args and `RETURNING updated_at`.
   - Pagination is always bounded: `repository.Pagination` clamps page into 1..`MaxPage` and limit into 1..`MaxLimit` (default `DefaultLimit` = 20) in `ApplyPagination`, so a request with no query string still gets a `LIMIT`. Never hand raw request values to `Paginate`. The entity filter exposes one `Apply(repository.Query)` method (pointer receiver — it must persist those clamps) and `Pagination.Meta(total)` reports the result using integer page math `(total + limit - 1) / limit`.
   - Reads: explicit `SELECT` with aliased columns (`FROM users AS u WHERE u.email=$1`) or `SELECT *` for full-struct scans; fetch via `pgxscan.Get(ctx, conn, &dst, query, args...)` / `pgxscan.Select(...)` for lists.
   - Dynamic list queries use the builder (`query_builder.go`): `Select(table, columns...)`, then sequential calls — `qb.WhereIn(…)`, `qb.WhereNull(…)`, `qb.OrderBy(…)`, `qb.Paginate(…)` — and finally `qb.SQL()` for the statement or `qb.CountSQL()` for the total. The modifiers implement `[repository.Query]` and return nothing, so they cannot be chained (`Select(…).WhereNull(…)` does not compile).

4. **Transaction awareness is automatic:** always resolve the connection via `conn := r.db.GetConn(ctx)`; when inside a service transaction it returns the tx. For reads that must lock, append `FOR UPDATE` when the connection is a tx:
   ```go
   if r.db.IsTxConn(conn) {
       query += "\nFOR UPDATE"
   }
   ```
   Never call `conn.Begin` inside a repository.

5. **Error translation via helpers** (`common.go`): `uniqueViolationErr(err, constraint)` → domain conflict error with details; `noRowsErr(err)` → `apperr.ErrNotFound` (optionally with details); anything else → `fmt.Errorf("short op: %w", err)`.

6. **Never return raw `sql.ErrNoRows`/`pgx.ErrNoRows`** to callers; always normalize to domain errors. Never log inside repositories — return the error instead.

## Checklist (Review mode)

- [ ] Constructor returns unexported type; `[Interface]` doc on every method
- [ ] nil guard on mutators; `NamedArgs` + shared builders; no string-concatenated SQL by hand
- [ ] `GetConn` used for every query; `FOR UPDATE` added on tx reads (via `IsTxConn`)
- [ ] No `Begin`/`Commit` inside repository; multi-step work left to service `Transact`
- [ ] unique/no-rows normalized; other errors wrapped with `fmt.Errorf("op: %w")`
- [ ] No business logic, no logging, no loops doing N queries where one JOIN/IN would do

## References

- `internal/infrastructure/postgres/common.go` — `generateInsertQuery`/`generateUpdateQuery` + error helpers
- `internal/infrastructure/postgres/query_builder.go` — `Select` builder (`WhereIn`/`WhereNull`/`OrderBy`/`Paginate`, `SQL`/`CountSQL`)
- `internal/infrastructure/postgres/user_repository.go` — canonical insert/update/select/delete
- `internal/infrastructure/postgres/auth_repository.go` — session/token repos with FOR UPDATE
- `internal/infrastructure/postgres/postgres.go` — `DB.GetConn`/`IsTxConn`
