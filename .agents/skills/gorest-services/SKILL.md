---
name: gorest-services
description: "Service-layer conventions for github.com/prawirdani/golang-restapi — interface-driven dependencies, Transact for multi-step writes, nullable mutation + Validate, audit entries inside the transaction, post-commit side effects (events, storage cleanup), async goroutines with snapshotted logger context, per-domain permission tables, and throttling. Use when writing or reviewing business logic in internal/ entity packages."
user-invocable: true
license: MIT
compatibility: Designed for AI coding agents working in the golang-restapi repository.
metadata:
  author: prawirdani
  version: "1.0.0"
  module: github.com/prawirdani/golang-restapi
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Agent
---

**Persona:** You are the business-logic reviewer. Services are the only place for orchestration. They own transactions, invariants, and side effects; handlers and repositories must stay dumb.

## Rules

1. **Dependencies are interfaces from the domain layer** (`repository.Transactor`, `user.Repository`, `storage.Storage`, `throttle.Throttler`, `Mailer`), injected via `NewService`. Services never import `postgres`, `redis`, or `r2` packages — the concrete types arrive via the interface.

2. **Multi-step writes wrap in `s.transactor.Transact(ctx, func(ctx) error {...})`.**    Repositories join the tx automatically through `db.GetConn(ctx)` — no explicit begin/commit. Examples: `Register` and `Login` in `internal/auth/service.go`.

3. **Side effects that must survive rollback happen AFTER commit, outside the closure.** Notification events are produced only once `Transact` returns nil (`s.eventProducer.ProduceRegistrationCompletionEvent`, `ProducePasswordRecoveryEvent` in `auth/service.go`); storage cleanup runs after the DB swap succeeds (`s.asyncDeleteImage` in `user/service.go`).

4. **Mutate domain models through their methods/fields, then validate:**
   ```go
   usr.Name = input.Name
   usr.Phone.Set(input.Phone, false)
   if err := usr.Validate(); err != nil { return err }
   return s.userRepo.Update(ctx, usr)
   ```
   Nullable columns use `pkg/nullable` (`Set(value, false)` / `NotNull()` / `Get()`); leave the nullable unset instead of storing zero values when the field wasn't provided.

5. **Async cleanup goroutines must snapshot the logger and use a fresh context:**
   ```go
   logger := log.GetFromContext(ctx).With("image_path", path, "reason", reason)
   go func() {
       cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
       defer cancel()
       ...
   }()
   ```
   Never reuse the request `ctx` in a goroutine that outlives the request (see `asyncDeleteImage` in `user/service.go`).

6. **Throttle before expensive work:** `s.throttler.TryAcquire(ctx, fmt.Sprintf("recover-password:%s", inp.Email), PasswordRecoveryThrottledTTL)`; on `!result.Allowed` return the throttled error `ErrPasswordRecoveryThrottled.WithDetails(result)`; on throttler error, propagate the error rather than guessing. (Check `internal/ports/throttle` for the interface contract.)

7. **Audits record state changes, inside the change's transaction.** Call `s.audit.Record(ctx, ...)` from inside the same `Transact` closure as the mutation, so a committed action is never silently unaudited. Refused attempts and token-reuse signals are **security events, not state changes**: emit `log.WarnCtx` instead of an audit row. There is no best-effort audit path, and a refused login must not write.

8. **Permissions are declared per domain, only where they are enforced.** Each entity owns its `Perm*` constants and its package-level `permTables` (`rbac.PermissionTable`), registered from `NewService` via `authorizer.RegisterPermissions`. Register unconditionally — grants are data; *enforcement* (`Require`/`RequireSelfOr`, or route middleware) is what depends on config such as `APP_INTERNAL_MODE`. Never declare a permission with no enforcement point, and never register another domain's permissions.

9. **Errors bubble up unchanged** — services return the domain error from repos/helpers; only log where context is added (`log.ErrorCtx(ctx, "...", err)` before returning is optional, avoid log-and-return pairs).

## Checklist (Review mode)

- [ ] Service struct fields are interfaces, not concrete infrastructure types
- [ ] Multi-step writes inside one `Transact`; post-commit side effects outside it
- [ ] Email/storage cleanup failure after commit logged, not fatal (or handled explicitly)
- [ ] Nullable fields via `pkg/nullable`; model `Validate()` before persisting
- [ ] Goroutines use fresh `context.Background()` + snapshot logger
- [ ] Rate-limited operations call `TryAcquire` first with a namespaced key
- [ ] Audit entries written inside the same transaction as the change; refused attempts logged, not audited
- [ ] Permissions declared beside the service and registered in `NewService`, each with an enforcement point
- [ ] No DB/SQL, no HTTP, no JSON marshaling in the service layer

## References

- `internal/auth/service.go` — `Transact`, audit-in-tx, throttling, post-commit event production, `permTables`
- `internal/user/service.go` — validation, storage swap + async cleanup, `permTables`
- `internal/user/model.go` / `gender.go` — nullable + Validate patterns
- `internal/ports/throttle/throttle.go` — Throttler contract
- `pkg/log/context.go` — logger snapshots (`log.GetFromContext`)
