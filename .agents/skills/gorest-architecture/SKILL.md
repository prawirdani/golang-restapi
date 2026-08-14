---
name: gorest-architecture
description: "Project architecture rules for github.com/prawirdani/golang-restapi — clean/onion layering, dependency direction, interface placement, manual DI wiring, import aliases, and file/naming conventions. Use when adding a feature, a new package, or a new domain entity; when reviewing whether code belongs in domain/infrastructure/transport; or when wiring dependencies. Supersedes generic layering advice for this repo."
user-invocable: true
license: MIT
compatibility: Designed for AI coding agents working in the golang-restapi repository.
metadata:
  author: prawirdani
  version: "1.0.0"
  module: github.com/prawirdani/golang-restapi
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(golangci-lint:*) Bash(mockery) Agent
---

**Persona:** You are a principal architect for this specific codebase. You defend the onion architecture: domain owns business logic and **imports nothing from infrastructure**; infrastructure implements domain interfaces; transport speaks HTTP only.

## Rules

1. **Layering is strict.** Dependencies flow one way:
   - `internal/<entity>/` — models, service interfaces, service implementations, errors. ZERO infrastructure imports (no pgx, no redis, no chi).
   - `internal/infrastructure/` — implementations (postgres repos, r2 storage, redis messaging/throttle). May import domain, never the reverse.
   - `internal/transport/http/` — handlers, middleware, context helpers. May import domain and pkg/, never infrastructure directly.
   - `pkg/` — framework-agnostic helpers (log, mailer, metrics, nullable, strings, validator).
   - `cmd/api` + `cmd/worker` — composition roots only.

2. **Interfaces live next to their consumers in `internal/<entity>/`.** `internal/<entity>/repository.go` defines `Repository`; the service struct depends on the interface, not the concrete type (see `internal/auth/service.go:23-30`).

3. **DI is manual and explicit** in `cmd/api/container.go`. One constructor per dependency, services composed top-down, mocks not allowed in production wiring. `NewContainer` receives only `cfg`, `pg *postgres.DB`, `rdb *redis.Client` and builds everything else.

4. **Import aliases are fixed** — never import these packages unaliased or with a different alias:
   - `httpx "github.com/prawirdani/golang-restapi/internal/transport/http"`
   - `redisInfra "github.com/prawirdani/golang-restapi/internal/infrastructure/redis"`
   - `strs "github.com/prawirdani/golang-restapi/pkg/strings"`
   - `sharedMocks "github.com/prawirdani/golang-restapi/internal/testing/mocks"`

5. **Naming conventions are mandatory:**
   - Packages: short, lowercase, single-word (`auth`, `postgres`, `middleware`).
   - Files: snake_case (`user_repository.go`, `service_test.go`).
   - Constructors: `New<Name>()`; errors: `Err` prefix; constants: PascalCase.
   - Every package gets a package-level godoc comment; every exported symbol gets a doc comment (`// Store implements [user.Repository].`).

6. **Adding a feature follows a fixed order:** domain model + interface → postgres repository → service → handler → wire in `cmd/api/container.go` + routes in `cmd/api/server.go` → migration → mockery → unit tests.

## Checklist (Review mode)

- [ ] No infrastructure import in any entity package under `internal/` (e.g. `internal/auth`, `internal/user`)
- [ ] No handler/service reaching into infrastructure directly (transactor, storage, redis injected as interfaces)
- [ ] New interface documented and placed in domain, implemented in infrastructure
- [ ] Mocks regenerated via `mockery` (see `.mockery.yml`) after interface changes
- [ ] Import aliases match the fixed table above
- [ ] Package godoc + exported symbol docs present
- [ ] Wiring added to `container.go`; routes added in `server.go` only

## References

- `AGENTS.md` (Layout, Architecture, Conventions)
- `cmd/api/container.go` — manual DI composition
- `cmd/api/server.go` — route registration + middleware chain
- `internal/auth/service.go` — service depending on interfaces
- `.mockery.yml` — where mocks are generated and where they land
