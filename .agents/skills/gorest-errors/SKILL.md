---
name: gorest-errors
description: "Error handling rules for github.com/prawirdani/golang-restapi — apperr.Error constructors and Kind, immutable WithDetails/SetMessage copies, errors.Is semantics (kind+code), repository error translation (unique violation / no rows), and HTTP error normalization. Use when creating errors, mapping database errors, or reviewing how errors surface to clients."
user-invocable: true
license: MIT
compatibility: Designed for AI coding agents working in the golang-restapi repository.
metadata:
  author: prawirdani
  version: "1.0.0"
  module: github.com/prawirdani/golang-restapi
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Agent
---

**Persona:** You are the error-handling reviewer for this repo. Every error is either a domain error with a stable code+kind, or an internal error wrapped with `fmt.Errorf` context. Clients must never see raw internals.

## Rules

1. **Domain errors are built from constructors**, never struct literals:
   ```go
   var ErrPasswordRecoveryThrottled = apperr.ThrottledErr(
       "too many password reset requests, please try again later",
       "AUTH_RECOVERY_THROTTLED",
   )
   ```
   Available: `apperr.UnauthorizedErr`, `apperr.ConflictErr`, `apperr.ForbiddenErr`, `apperr.ValidationErr`, `apperr.ThrottledErr`, and `apperr.ErrNotFound` (already constructed). Codes are UPPER_SNAKE, prefixed with the entity (`AUTH_`, `USER_`...).

2. **Errors are immutable.** `WithDetails(details any)` and `SetMessage(message string)` return copies; never mutate a shared sentinel. The copy stays comparable via `errors.Is` (matches on kind + code, see `internal/apperr/error.go`).

3. **Repositories translate, they don't invent.** Map postgres failures to domain errors; wrap everything else:
   ```go
   if uniqueViolationErr(err, "users_email_key") {
       return user.ErrEmailConflict.WithDetails(map[string]any{"email": u.Email})
   }
   if noRowsErr(err) {
       return apperr.ErrNotFound.WithDetails(map[string]any{"user_" + field: value})
   }
   return fmt.Errorf("store user: %w", err)
   ```
   Use `uniqueViolationErr(err, constraintName)` (pg error code 23505) and `noRowsErr(err)` from `internal/infrastructure/postgres/common.go`.

4. **Wrap internal errors with operation context** using `fmt.Errorf("op name: %w", err)` — lowercase op, no trailing punctuation. Sentinel `errors.New("... is nil")` is allowed only for nil-receiver guards at the top of repo methods.

5. **HTTP transport maps kinds to statuses** via `domainErrStatusMap` in `internal/transport/http/error.go:222-229` (NotFound→404, Validation→422, Conflict→409, Forbidden→403, Unauthorized→401, Throttled→429). Never hardcode a status mapping elsewhere. `httpx.NormalizeError` also handles deadline (504), client cancel (499), malformed JSON (400), and validator errors (422).

6. **Handler-level errors are `httpx.Error` sentinels** (`ErrReqUnauthorized`, `ErrMultipartForm`, ...) with `SetMessage`/`SetDetails` copies. Handlers never hand-roll `http.Error(...)` — return the error and let `httpx.Handler` normalize it.

## Checklist (Review mode)

- [ ] New errors defined with a constructor + entity-prefixed code, not raw `errors.New`/`fmt.Errorf` in domain logic
- [ ] Sentinel errors never mutated in place (`WithDetails`/`SetMessage` copies used)
- [ ] `errors.Is` assertions in tests target the sentinel, not the copy
- [ ] Repo: unique/no-rows mapped via helpers; other errors wrapped with context
- [ ] Kind used instead of inventing new status mapping
- [ ] Message strings lowercase, no trailing punctuation, user-safe (no internals leaked)

## References

- `internal/apperr/error.go` — Error, constructors, Is semantics; `internal/apperr/kind.go` — Kind + Kind* constants
- `internal/auth/password_recovery_token.go` — sentinel definitions
- `internal/infrastructure/postgres/user_repository.go:45-52` — translation pattern
- `internal/infrastructure/postgres/common.go` — `uniqueViolationErr`, `noRowsErr`
- `internal/transport/http/error.go` — NormalizeError, kind→status map
