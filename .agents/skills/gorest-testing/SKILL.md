---
name: gorest-testing
description: "Unit testing conventions for github.com/prawirdani/golang-restapi — mockery regeneration and placement, setupTestFixture pattern, mock.AssertExpectations cleanup, transactor mock expectations inside RunAndReturn, table-driven subtests, and assertion style (assert vs require). Use when writing or reviewing unit tests in internal/ (entity packages) or internal/transport/http/."
user-invocable: true
license: MIT
compatibility: Designed for AI coding agents working in the golang-restapi repository.
metadata:
  author: prawirdani
  version: "1.0.0"
  module: github.com/prawirdani/golang-restapi
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Bash(mockery) Agent
---

**Persona:** You are a test-quality reviewer. Unit tests must be deterministic, mock everything at the interface boundary, assert exact call contracts, and never touch real databases/Redis/R2.

## Rules

1. **Mocks come from mockery only** (`.mockery.yml`):
   - Entity-scoped: `internal/<entity>/mocks/` — `auth/mocks/{repository,user_repository,event_producer}.go`, `user/mocks/repository.go`.
   - Shared infrastructure: `internal/testing/mocks/` (`Transactor`, `Storage`, `File`, `Throttler`).
   - After adding/changing an interface, run `mockery` and commit the regenerated files. Never hand-write mock implementations.

2. **Fixture pattern is fixed** (see `setupTestFixture` in `internal/auth/service_test.go` and `internal/user/service_test.go`):
   ```go
   type testFixture struct {
       transactor *sharedMocks.Transactor
       userRepo   *mocks.UserRepository
       ...
       service    *auth.Service
       cfg        config.Auth
   }

   func setupTestFixture(t *testing.T) *testFixture {
       cfg := config.Auth{...}
       tr := sharedMocks.NewTransactor(t)
       ...
       service := auth.NewService(cfg, tr, userRepo, authRepo, mailer, throttler)
       t.Cleanup(func() {
           tr.AssertExpectations(t)
           userRepo.AssertExpectations(t)
           ...
       })
       return &testFixture{...}
   }
   ```
   - Mocks are constructed with `t` so unexpected calls fail the test automatically.
   - `t.Cleanup` runs `AssertExpectations` for every mock.

3. **Transactor expectations are declared inside `RunAndReturn`** because expectations must be registered before the closure runs:
   ```go
   f.transactor.EXPECT().
       Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
       RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
           f.authRepo.EXPECT().GetSessionByRefreshTokenHash(ctx, mock.AnythingOfType("[]uint8")).Return(session, nil)
           f.authRepo.EXPECT().UpdateSession(ctx, session).Return(nil)
           return fn(ctx)
       })
   ```
   `mock.AnythingOfType` for closures, domain structs (`"*auth.Session"`), and hashed bytes (`"[]uint8"`).

4. **Test structure:** `TestService_<Method>` with `t.Run("Case name", ...)` subtests, one behavior per subtest. Pure function tests (token generation, hashing, JWT) live in the same package with plain assertions.

5. **Assertion style:** `assert` for continuing checks (`assert.NoError`, `assert.ErrorIs`), `require` for setup that must not continue (`require.NoError(t, err)` before using a value). Error identity checks use `assert.ErrorIs(t, err, sentinel)` — sentinels, not copies.

6. **Negative-path rigor:** when a failure must prevent later calls, rely on the mock's unexpected-call failure (constructed with `t`) instead of asserting call counts manually. A test that proves "mailer never called" simply registers no mailer expectation.

7. **Tests must be config-independent.** Never require a local `.env`: config tests set what they need with `t.Setenv` (`config/config_test.go`), and the whole suite must pass with `.env` absent. Tests are deterministic — no sleeps, no real time dependencies (`time.Now()` offsets are fine).

8. **Run with the full suite:** `make test` (`go test -race -count=1 ./... -cover`) and `make lint` before finishing. Tests under `internal/transport/http/` (`middlewares_test.go`, `metrics_integration_test.go`) exercise middleware and do not use the entity fixture.

## Checklist (Review mode)

- [ ] Mocks regenerated (not hand-written) and committed with the interface change
- [ ] Fixture via `setupTestFixture` + `t.Cleanup` AssertExpectations for all mocks
- [ ] Transactor inner expectations registered inside `RunAndReturn`
- [ ] Subtests named for behavior; assert/require used appropriately
- [ ] `ErrorIs` against sentinels; negative paths proven by absent expectations
- [ ] `make lint && make test` green without a local `.env`

## References

- `internal/auth/service_test.go` — canonical fixture (`setupTestFixture`) + transactor pattern
- `internal/user/service_test.go` — second example
- `config/config_test.go` — `t.Setenv` config isolation
- `.mockery.yml` — mock placement and regeneration config
- `internal/testing/mocks/` — shared infrastructure mocks
