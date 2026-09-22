# Golang REST API

A production-shaped REST API template in Go. It ships an opinionated clean/onion
architecture (Handler → Service → Repository), stateless JWT access tokens with
server-side sessions, Redis Streams for asynchronous email delivery, PostgreSQL
via `pgx` with raw SQL, Cloudflare R2 object storage, and Prometheus metrics.

The goal is a reference implementation you can rename and extend: strict layer
boundaries, interface-driven dependencies, manual dependency injection, and
tests that never touch a real database, Redis, or SMTP server.

- Module: `github.com/prawirdani/golang-restapi`
- Go: `1.26.5` (see `go.mod`)
- License: MIT (see `LICENSE`)

---

## Table of contents

- [Highlights](#highlights)
- [Tech stack](#tech-stack)
- [Architecture](#architecture)
  - [Layers and dependency direction](#layers-and-dependency-direction)
  - [Where interfaces live](#where-interfaces-live)
  - [Dependency injection](#dependency-injection)
  - [Request flow](#request-flow)
- [Project layout](#project-layout)
- [Getting started](#getting-started)
- [Configuration](#configuration)
  - [Startup validation](#startup-validation)
- [Authentication and sessions](#authentication-and-sessions)
  - [Access token](#access-token)
  - [Refresh token and session](#refresh-token-and-session)
  - [Token delivery and cookies](#token-delivery-and-cookies)
  - [Access-token revocation](#access-token-revocation)
  - [Registration (invitation flow)](#registration-invitation-flow)
  - [Password recovery](#password-recovery)
  - [Password change](#password-change)
  - [Throttling](#throttling)
  - [Passwords and token entropy](#passwords-and-token-entropy)
- [Authorization (RBAC)](#authorization-rbac)
- [API reference](#api-reference)
  - [Public routes](#public-routes)
  - [Authenticated routes](#authenticated-routes)
  - [List query DSL](#list-query-dsl)
  - [Response envelope](#response-envelope)
  - [curl examples](#curl-examples)
- [Error handling](#error-handling)
- [Audit logging](#audit-logging)
- [Messaging and the worker](#messaging-and-the-worker)
- [Observability and health](#observability-and-health)
- [Database and migrations](#database-and-migrations)
- [Testing](#testing)
- [Development tooling](#development-tooling)
- [Conventions](#conventions)
- [License](#license)

---

## Highlights

- **Clean/onion layering** with a one-way dependency graph: domain packages know
  nothing about PostgreSQL, Redis, R2, or Fiber; infrastructure implements the
  interfaces the domain declares.
- **Split-token auth**: HS256 JWT access tokens (short TTL) plus opaque,
  rotating refresh tokens persisted server-side as SHA-256 hashes.
- **Access-token revocation without stateful tokens**: a Redis marker store
  invalidates stateless JWTs for logout, single-session revoke, password
  change/reset, admin wipe, and user deletion.
- **Invitation-based registration**: no account row exists until the invitee
  consumes the emailed, single-use token and sets a password.
- **Async email delivery** over Redis Streams with consumer groups, a pending
  entries list (PEL), idempotent dedup, dead-letter queue, and panic isolation.
- **Audit trail** written inside the same transaction as the change it records.
- **Typed error kernel** (`apperr`) mapped to HTTP status codes in one place.
- **Generic list query DSL**: pagination, allow-listed sorting, enum-validated
  filters, and the applied query echoed back in the response `meta`.
- **Hermetic unit tests** using `testify` + `mockery`; no real infrastructure.

## Tech stack

| Concern | Choice |
| --- | --- |
| Language | Go 1.26.5 |
| HTTP | [Fiber v3](https://github.com/gofiber/fiber) (`github.com/gofiber/fiber/v3` v3.5.0) |
| Database | PostgreSQL via [pgx v5](https://github.com/jackc/pgx) (raw SQL, no ORM) + [scany/v2](https://github.com/georgysavva/scany) for struct scanning |
| Migrations | [goose](https://github.com/pressly/goose) (SQL files in `migrations/`) |
| Cache / queue / throttle | Redis via [go-redis v9](https://github.com/redis/go-redis): Streams, `SET NX` throttle, revocation markers |
| Object storage | Cloudflare R2 through the AWS SDK v2 S3 client |
| Auth | [golang-jwt/jwt v5](https://github.com/golang-jwt/jwt), `golang.org/x/crypto/bcrypt` |
| Validation | [go-playground/validator v10](https://github.com/go-playground/validator) |
| Config | `.env` via [godotenv](https://github.com/joho/godotenv) |
| Logging | stdlib `log/slog` and [zerolog](https://github.com/rs/zerolog) behind a swappable `log.Logger` |
| Metrics | [Prometheus client_golang](https://github.com/prometheus/client_golang) |
| Mail | [gomail.v2](https://pkg.go.dev/gopkg.in/gomail.v2) |
| Testing | [testify](https://github.com/stretchr/testify) + [mockery](https://github.com/vektra/mockery) |
| Tooling | [Air](https://github.com/cosmtrek/air) hot reload, [golangci-lint v2](https://github.com/golangci/golangci-lint) |

Direct dependencies (from `go.mod`) include `github.com/aws/aws-sdk-go-v2`
(plus `config`, `credentials`, `service/s3`), `github.com/georgysavva/scany/v2`,
`github.com/go-playground/validator/v10`, `github.com/gofiber/fiber/v3`,
`github.com/golang-jwt/jwt/v5`, `github.com/google/uuid`, `github.com/jackc/pgx/v5`,
`github.com/joho/godotenv`, `github.com/prometheus/client_golang`,
`github.com/redis/go-redis/v9`, `github.com/stretchr/testify`, `golang.org/x/crypto`,
`golang.org/x/sync`, `golang.org/x/text`, `gopkg.in/gomail.v2`, and
`github.com/rs/zerolog`.

## Architecture

The codebase follows a clean/onion model. Each layer has a single
responsibility and may only depend on the layer beneath it.

> **Handler** (HTTP only) → **Service** (business logic) → **Repository** (data access)

### Layers and dependency direction

| Layer | Location | Responsibility | May import |
| --- | --- | --- | --- |
| Transport | `internal/transport/http` | Parse/validate requests, call services, serialize responses, cookies, middleware | Domain packages, `pkg/` |
| Domain (entities) | `internal/auth`, `internal/user`, `internal/audit`, `internal/rbac` | Models, business rules, service implementations, consumer-side interfaces, errors | Other domain packages and `internal/ports/*`; **never** `internal/infrastructure/*` or Fiber |
| Ports | `internal/ports/*` | Interfaces the domain and infrastructure agree on (transactor, storage, throttle, messaging, revocation) | Stdlib and small value types only |
| Infrastructure | `internal/infrastructure/*` | Concrete implementations: `postgres`, `redis`, `r2` | Domain/ports |
| Shared | `pkg/*` | Framework-agnostic helpers (`log`, `mailer`, `metrics`, `nullable`, `strings`, `validator`) | Stdlib and third-party libs |
| Composition roots | `cmd/api`, `cmd/worker`, `cmd/cli` | Wire everything, start processes, expose routes | Everything |

The rule that keeps this honest: **entity packages import no infrastructure.**
A service depends on the interface it needs, and the concrete implementation is
injected from the composition root.

### Where interfaces live

Interfaces are declared next to their consumers, split by use rather than one
wide interface:

- `internal/auth/interfaces.go` — `Repository` (composed from `SessionReader`,
  `SessionWriter`, `TokenReader`, `TokenWriter`), a narrow `UserRepository`, and
  `EventProducer`.
- `internal/user/repository.go` — `Repository` for user persistence. The service
  also declares a local `SessionRevoker` (implemented by the auth repository) so
  the user service can revoke sessions inside its own transaction without an
  import cycle into `internal/auth`.
- `internal/ports/repository` — `Transactor`, plus the `Query`/`Filterer`/
  `Sorter`/`Paginator` contracts used by the generic query builder.
- `internal/ports/storage` — `Storage`, `File` (implemented by R2 and by the
  multipart upload adapter).
- `internal/ports/throttle` — `Throttler`, `Result`.
- `internal/ports/messaging` — `Envelope[T]`, `Handler[T]`.
- `internal/ports/revocation` — `Checker`, `Revoker`, and their composition
  `Store`, implemented by the Redis revocation store.

### Dependency injection

`cmd/api/container.go` is the only place production dependencies are assembled.
`NewContainer(cfg, pg, rdb)` builds the R2 client, the Redis throttler and
revocation store, the PostgreSQL repositories, the in-memory authorizer, and
then the services (`user.Service`, `auth.Service`, `audit.Service`). The
resulting `*Container` exposes the services plus the revocation store that the
auth middleware needs. `cmd/api/server.go` then constructs handlers and the
authenticator middleware and mounts routes. There is no DI framework and no
service locator.

### Request flow

For an authenticated request such as `GET /api/auth/me`:

1. Global middleware runs, in order: production rate limit (production only),
   panic recoverer, request-metrics instrumentation, security headers, request
   logger, request-id, audit-context injection, compression, weak ETag, CORS.
2. chi-style routing resolves the route; the `authenticatorMiddleware` runs for
   protected routes. It reads the access token from the `access_token` cookie or
   `Authorization: Bearer`, verifies the JWT signature/`exp`/`iat`/`sid`, checks
   the Redis revocation markers, and injects the RBAC actor + session id and
   request-scoped log fields into the request context.
3. The handler (`AuthHandler.getCurrentUser`) pulls the actor from context,
   calls `user.Service.GetUserByID`, and the service enforces
   `RequireSelfOr(user.read)`.
4. The repository calls `db.GetConn(ctx)`, which returns the pool or, inside a
   transaction, the `pgx.Tx` stored in the context. Inside a transaction the
   repository adds `FOR UPDATE` to the read.
5. The handler returns `Body{Data: ...}`; Fiber serializes it. Any returned
   error goes through the `ErrorHandler` → `ParseError` → status mapping.

## Project layout

```text
cmd/
  api/                 HTTP server entrypoint (Fiber)
    main.go            config, postgres, redis, graceful shutdown
    container.go       manual DI wiring
    server.go          middleware chain, routes, health, metrics exporter
  worker/              background worker entrypoint (consumes Redis Streams)
  cli/                 developer CLI (`permissions` subcommand, extensible)

config/                env-based config structs, parsed and validated at startup
  config.go            LoadConfig + Validate
  app.go postgres.go redis.go cors.go auth.go smtp.go r2.go proxy.go

internal/
  apperr/              typed error kernel: Kind, Error, constructors
  audit/               audit Entry, request context, Recorder/Reader, service
  auth/                access tokens, sessions, crypto, models, service
    mocks/             entity-scoped auth mocks
  user/                user model, service, repository interface, search DSL
    mocks/             entity-scoped user mocks
  rbac/                roles, permissions, authorizer, actor context
  ports/               interfaces implemented by infrastructure
    messaging/         message envelope + handler
    repository/        Transactor, Query, Sorting, Pagination
    revocation/        access-token revocation ports (Checker/Revoker/Store)
    storage/           object storage (Storage, File)
    throttle/          request throttling (Throttler, Result)
  infrastructure/
    postgres/          pgx pool, transaction wrapper, query builder, repositories
    redis/             Streams producer/consumer, throttle, revocation store
    r2/                Cloudflare R2 storage (S3 API)
  transport/
    http/              Fiber handlers, middleware, error normalization, router
  worker/              AuthWorker: renders and sends auth emails
  testing/mocks/       shared infrastructure mocks (Transactor, Storage, ...)

pkg/                   framework-agnostic helpers
  log/                 swappable Logger (slog + zerolog adapters), context logging
  mailer/              gomail wrapper with a bounded send
  metrics/             Prometheus registry + Fiber instrumentation middleware
  nullable/            nullable column helper
  strings/             string helpers
  validator/           validation kernel over go-playground/validator

migrations/            goose SQL migrations (additive only)
deployment/
  nginx/               nginx reverse-proxy config used by compose
  prometheus/          scrape config
  grafana/             datasource/dashboard provisioning + api-metrics.json

compose.yml            api + nginx + prometheus + grafana
Dockerfile             multi-stage build (distroless runtime)
Makefile               dev/build/test/lint/migration/cli targets
```

## Getting started

### Prerequisites

- Go `1.26.5` or newer (matching `go.mod`).
- PostgreSQL and Redis reachable from the process.
- The [goose](https://github.com/pressly/goose) CLI on your `PATH` for the
  `make migration:*` targets.
- Optional: [Air](https://github.com/cosmtrek/air) for `make dev` /
  `make dev:worker`, and `golangci-lint` v2 for `make lint`.

### 1. Start supporting services

`compose.yml` runs the API image plus nginx, Prometheus, and Grafana. Postgres
and Redis are expected to be reachable from the API container (the compose file
adds `host.docker.internal:host-gateway` so a host-run Postgres works). If you
prefer to run only the datastores, start Postgres and Redis directly.

```bash
docker compose up -d
```

### 2. Configure the environment

```bash
cp .env.example .env
# edit .env — at minimum set AUTH_JWT_SECRET (>= 32 chars) and DB_* values
```

`config.LoadConfig()` calls `godotenv.Load()`, so a local `.env` is read
automatically in development. `AUTH_JWT_SECRET` is required and must be at
least 32 characters; startup fails otherwise.

### 3. Apply migrations

```bash
make migration:up
```

Migrations are additive only; see [Database and migrations](#database-and-migrations).

### 4. Run the API and the worker

They are separate processes:

```bash
make dev          # API server with hot reload (Air)
make dev:worker   # stream consumer with hot reload (Air)
```

### 5. Build a binary

```bash
make build        # CGO_ENABLED=0 GOOS=linux -> ./bin/api
make run          # run ./bin/api
```

`make build` produces a static Linux binary at `./bin/api`; the `Dockerfile`
copies that binary into a distroless runtime image and runs it.

## Configuration

All configuration comes from environment variables, optionally loaded from
`.env`. Parsing happens in `config/*.go`; `config.LoadConfig()` parses every
section and then calls `Config.Validate()`.

### Environment variables

**Application**

| Variable | Purpose | Code default | Notes |
| --- | --- | --- | --- |
| `APP_NAME` | Service name | empty | Informational |
| `APP_VERSION` | Version string | empty | Label on `app_info` metric |
| `APP_PORT` | API listen port | empty (0) | Non-integer aborts startup. API binds `127.0.0.1:<port>` |
| `APP_ENV` | Environment | empty | Must be `dev` or `prod`; anything else fails validation |
| `APP_INTERNAL_MODE` | Registration becomes admin-only | `false` | Parsed with `strconv.ParseBool` |

**PostgreSQL**

| Variable | Purpose | Code default | Notes |
| --- | --- | --- | --- |
| `DB_USER` | DB user | empty | |
| `DB_PASSWORD` | DB password | empty | |
| `DB_HOST` | DB host | empty | |
| `DB_PORT` | DB port | empty (0) | |
| `DB_NAME` | DB name | empty | |
| `DB_MINCONNS` | Pool minimum connections | `0` | Must be `>= 0` and `<= DB_MAXCONNS` |
| `DB_MAXCONNS` | Pool maximum connections | **required** | Must be `> 0` and `<= 2147483647` |
| `DB_MAXCONN_LIFETIME` | Max connection lifetime | empty (0) | Go duration (e.g. `60m`); zero keeps the pgx default |

The pool also hardcodes `MaxConnIdleTime = 5m` and `HealthCheckPeriod = 1m`;
these are not configurable.

**Redis**

| Variable | Purpose | Code default | Notes |
| --- | --- | --- | --- |
| `REDIS_HOST` | Redis host | empty | |
| `REDIS_PORT` | Redis port | empty (0) | |
| `REDIS_PASSWORD` | Redis password | empty | |

The Redis client always uses DB index `0` and there is no connection-pool size
setting — neither is configurable through env.

**CORS and proxy**

| Variable | Purpose | Code default | Notes |
| --- | --- | --- | --- |
| `CORS_ORIGINS` | Comma-separated allowed origins | empty | With credentials enabled, `*` or an unparseable origin fails startup |
| `CORS_CREDENTIALS` | Send credentials | `false` | |
| `TRUSTED_PROXIES` | Comma-separated IPs/CIDRs of proxies whose `X-Forwarded-For`/`X-Real-IP` are trusted | empty | An invalid entry aborts startup; when empty, forwarded headers are ignored |

**Auth**

| Variable | Purpose | Code default | Notes |
| --- | --- | --- | --- |
| `AUTH_JWT_SECRET` | HS256 signing key | **required** | Must be at least 32 characters; rotating it invalidates all access tokens |
| `AUTH_JWT_TTL` | Access-token lifetime | `15m` | Short TTL bounds revocation exposure (revocation checks fail open) |
| `AUTH_REVOCATION_FAIL_CLOSED` | Deny access when the revocation store errors | `false` | When false the middleware fails open and logs a warning |
| `AUTH_SESSION_TTL` | Session / refresh-token lifetime | empty (0) | `.env.example` ships `168h` (7 days) |
| `AUTH_PASSWORD_RECOVERY_TOKEN_TTL` | Reset-token lifetime | `5m` | `.env.example` ships `15m`; the short code default limits `?token=` URL exposure |
| `AUTH_REGISTRATION_TOKEN_TTL` | Registration-token lifetime | `15m` | Same URL-exposure reasoning |
| `AUTH_RESET_PASSWORD_FORM_ENDPOINT` | Web UI URL for the reset form | empty | Used to build the reset link |
| `AUTH_COMPLETE_REGISTRATION_FORM_ENDPOINT` | Web UI URL for the password-creation form | empty | Used to build the invite link |

**SMTP**

| Variable | Purpose | Code default | Notes |
| --- | --- | --- | --- |
| `SMTP_HOST` | SMTP host | empty | |
| `SMTP_PORT` | SMTP port | empty (0) | |
| `SMTP_SENDER_NAME` | `From` header | empty | e.g. `Example <example@mail.com>` |
| `SMTP_AUTH_EMAIL` | SMTP username | empty | |
| `SMTP_AUTH_PASSWORD` | SMTP password | empty | |

**Cloudflare R2**

| Variable | Purpose | Code default | Notes |
| --- | --- | --- | --- |
| `R2_BUCKET_URL` | Public bucket base URL | empty | Leave empty for a private bucket |
| `R2_BUCKET` | Bucket name | empty | |
| `R2_ACCOUNT_ID` | Cloudflare account id | empty | Builds the R2 S3 endpoint |
| `R2_ACCESS_KEY_ID` | R2 access key | empty | |
| `R2_ACCESS_KEY_SECRET` | R2 secret | empty | |

### Startup validation

Validation is split across the section parsers and `Config.Validate()`:

| Check | Where | Failure |
| --- | --- | --- |
| `APP_ENV` is `dev` or `prod` | `Config.Validate()` | startup error |
| `AUTH_JWT_SECRET` length `>= 32` | `Config.Validate()` | startup error |
| `CORS_CREDENTIALS=true` with a `*` or invalid origin | `Config.Validate()` | startup error |
| `DB_MAXCONNS > 0` and `<= MaxInt32` | `Postgres.Parse()` | startup error |
| `DB_MINCONNS` within `[0, DB_MAXCONNS]` | `Postgres.Parse()` | startup error |
| `TRUSTED_PROXIES` entries parse as IP/CIDR | `Proxy.Parse()` | startup error |
| `APP_PORT` is an integer | `App.Parse()` | startup error |

Duration and integer env vars that fail to parse are silently ignored (the
field keeps its default) — except `APP_PORT`, which returns the parse error.

## Authentication and sessions

The auth design is a split-token scheme: short-lived stateless JWT access
tokens plus long-lived opaque refresh tokens backed by server-side session rows.

### Access token

- JWT signed with **HS256** using `AUTH_JWT_SECRET`.
- Claims: `sub` (user id, parsed into `UserID`), `sid` (session id), `role`, plus
  registered `iat` and `exp`.
- `exp` is required (`jwt.WithExpirationRequired()`) — a token without an
  expiry would make revocation TTLs meaningless.
- A token missing `iat` or `sid`, or with an unparseable/unknown signing method,
  is rejected as `AUTH_INVALID`. Expired tokens are rejected as `AUTH_EXPIRED`.
  Both map to **401**, never 500.
- Delivered in the `access_token` cookie or the `Authorization: Bearer` header.
  It is also echoed in the login/refresh response body.

### Refresh token and session

- 256-bit random opaque token, base64url-encoded with an `rt_` prefix.
- Only its SHA-256 hash is stored, in `sessions.refresh_token_hash` (unique).
- A session row holds `user_id`, `refresh_token_hash`, `user_agent`, `ip_addr`,
  `created_at`, `accessed_at`, `expires_at`, and `revoked_at`.
- **Rotation**: each refresh mints a new access token and a new refresh token,
  replaces the hash, and updates `accessed_at`/`ip_addr`/`user_agent` in one
  transaction. The old refresh token becomes unusable.
- **Reuse detection**: presenting a refresh token whose session is already
  revoked logs a WARN reuse signal (with user/session ids) and returns
  `AUTH_INVALID_SESSION` (401). There is no token-family history tracking.
- Expired sessions and revoked sessions are both rejected with the same 401.

### Token delivery and cookies

Login and refresh set both cookies via `setTokenCookies`:

| Attribute | Value |
| --- | --- |
| Names | `access_token`, `refresh_token` |
| `HttpOnly` | always on |
| `Secure` | production only |
| `SameSite` | `Lax` |
| `Path` | `/` |
| `Expires` | access = now + `AUTH_JWT_TTL`; refresh = now + `AUTH_SESSION_TTL` |

Logout clears both cookies.

### Access-token revocation

Access tokens are stateless, so a signature-valid token would otherwise stay
valid until it expires. To revoke before expiry, a Redis store holds two kinds
of **marker** (never a raw JWT or secret):

| Key | Value | Written by |
| --- | --- | --- |
| `revoked:user:<uid>` | `UnixNano` watermark of the revocation instant | user deletion, admin bulk wipe, password change/reset |
| `revoked:sess:<sid>` | `"1"` (presence) | logout, single-session revoke |

Both markers expire after `AUTH_JWT_TTL + 5s` (`revocationSkew`). If the
access-token TTL is non-positive the store treats tokens as already expired and
skips writes.

The auth middleware's check runs **after** `VerifyAccessToken` succeeds — never
before, since unverified claims are attacker-controlled — and performs a single
`MGET` of both keys, bounded by a **300 ms** timeout:

- Session marker present → revoked (session precedence).
- Otherwise the user watermark is parsed and compared: a token is revoked when
  `iat <= watermark + 5s`. Extending the revoked window past the watermark
  compensates for cross-instance clock skew and over-revokes, the safe
  direction. A brand-new login within 5 s of a revocation is briefly rejected
  too — deliberate.
- A corrupt or non-positive watermark returns an error rather than silently
  failing open.

Revoked tokens and fail-closed store errors both produce **401**
(`AUTH_INVALID_SESSION`). Expired and malformed tokens are also 401 but carry
their own codes (`AUTH_EXPIRED`, `AUTH_INVALID`), so the HTTP status never
reveals whether a token was revoked even though the `code` names the reason. On
a store error the middleware logs a WARN and **fails open by default**, because
the short access-token TTL bounds the exposure; set
`AUTH_REVOCATION_FAIL_CLOSED=true` to deny instead.

Where revocation is triggered:

| Action | Session rows | Access-token marker |
| --- | --- | --- |
| `DELETE /api/auth/logout` | revoke current session in tx | session marker after commit (best-effort; failure logged, still 200) |
| `DELETE /api/auth/sessions/:id` | revoke one session in tx | session marker after commit; failure is returned |
| `DELETE /api/auth/sessions/users/:userID` | revoke all user sessions in tx | user watermark after commit; failure is returned |
| `DELETE /api/users/:id` | revoke all user sessions in tx | user watermark after commit; failure is returned |
| `PUT /api/auth/password/change` | revoke all user sessions in tx | user watermark after commit (best-effort; still succeeds) |
| `PUT /api/auth/password/reset` | revoke all user sessions in tx | user watermark after commit (best-effort; still succeeds) |

### Registration (invitation flow)

Registration is invitation-based; no account exists until the invitee sets a
password.

1. `POST /api/auth/register` with `name` + `email`.
   - If the email already exists → `user.ErrEmailConflict` (409).
   - Any prior outstanding token for the email is revoked (latest invite wins;
     `revoked_at` is distinct from `used_at`).
   - A single-use token is created (256-bit, `regt_` prefix, SHA-256 stored,
     `AUTH_REGISTRATION_TOKEN_TTL`) and committed with an audit row.
2. After commit, a `email.user_registration` event is published to Redis
   Streams; the worker renders and sends the completion link.
3. `GET /api/auth/register/:token` lets the completion form inspect expiry,
   `used_at`, and `revoked_at`.
4. `POST /api/auth/register/complete` with `token` + `password` validates the
   token (missing/expired/revoked/used all return **401**
   `AUTH_INVALID_REGISTRATION_TOKEN`), hashes the password, creates the user,
   and marks the token used — all in one transaction. The password is hashed
   only after the token validates, so a garbage token cannot force an expensive
   bcrypt on this unauthenticated route.

When `APP_INTERNAL_MODE=true`, `POST /api/auth/register` is mounted behind
authentication and `auth.Service.Register` additionally requires the
`auth.register-user` permission (admin/system). The created user gets the
default `user` role and `email_verified_at` set.

### Password recovery

1. `POST /api/auth/password/recover` with `email` → per-email Redis throttle
   (30 s) → looks up the user.
2. A 256-bit opaque token is generated, its SHA-256 hash stored in
   `password_recovery_tokens` with a TTL (`AUTH_PASSWORD_RECOVERY_TOKEN_TTL`),
   and an audit row is written in the same transaction.
3. An `email.password_recovery` event is published; the worker sends the reset
   email.
4. `GET /api/auth/password/recover/:token` exposes status.
5. `PUT /api/auth/password/reset` with `token` + `new_password` consumes the
   token, updates the password, revokes all sessions, writes an audit row, and
   (after commit) waters the user's access tokens. A used/expired/unknown token
   returns **401**.

An unknown email propagates `apperr.ErrNotFound` → **404** (enumeration-as-feature,
kept deliberately).

### Password change

`PUT /api/auth/password/change` (authenticated) verifies the current password,
updates the password, revokes **all** sessions for the user (including the
current one), and writes an audit row. The user watermark is written after
commit as best-effort, so a Redis failure does not turn a completed password
change into an error.

### Throttling

| Scope | Mechanism | Limit |
| --- | --- | --- |
| Global | Fiber in-process limiter, **production only** | 20 req/min per IP |
| `POST /api/auth/login` | Fiber in-process limiter | 5 req/min per IP |
| `POST /api/auth/register/complete` | Fiber in-process limiter | 5 req/min per IP |
| `POST /api/auth/password/recover` | Fiber in-process limiter + Redis per-email throttle | 5 req/min per IP + 30 s per email (shared across instances) |

Login runs a dummy bcrypt comparison on unknown emails so response time does not
reveal whether an account exists.

### Passwords and token entropy

- Passwords are hashed with **bcrypt cost 12**.
- Password inputs validate `min=8,max=72` bytes; bcrypt truncates beyond 72.
- Refresh tokens: 32 random bytes (`rt_` prefix).
- Registration tokens: 32 random bytes (`regt` prefix).
- Password-recovery tokens: 32 random bytes (no prefix).

## Authorization (RBAC)

Authorization is role-based, code-defined, and in-memory. There are no
permission tables to keep in sync; each entity declares its own role→permission
table and registers it at startup from its `NewService` constructor.

- Roles (`rbac.Role`): `admin`, `user`, `system` (background/worker actions).
  The `users.role` column has a SQL `CHECK` constraint as a backstop.
- Permissions use the `"<entity>.<verb>"` grammar, e.g. `user.update`.
- Services enforce with `Require(ctx, perms...)` (the role must hold **all**
  given permissions) or `RequireSelfOr(ctx, userID, perm)` (passes if the actor
  is the target user, otherwise falls back to the permission check). The actor
  (`UserID`, `Role`) and `SessionID` are injected into the request context by the
  authenticator middleware.
- `RoleUser` holds no permissions in any table; a plain user reaches their own
  record only through `RequireSelfOr`.

The complete permission set and its grants:

| Permission | RoleAdmin | RoleSystem | RoleUser | Enforced by |
| --- | :---: | :---: | :---: | --- |
| `user.read` | yes | yes | no | `ListUser` (list) and `GetUserByID`/`GetUserByEmail` (`RequireSelfOr`) |
| `user.update` | yes | yes | no | `UpdateUser`, profile-picture change/delete (`RequireSelfOr`) |
| `user.delete` | yes | yes | no | `DeleteUser` (`Require`) |
| `audit.read` | yes | yes | no | `audit.Service.List` |
| `auth.change-password` | yes | yes | no | `ChangePassword` (`RequireSelfOr`) |
| `auth.register-user` | yes | yes | no | `Register` when `APP_INTERNAL_MODE=true` |
| `auth.revoke-user-sessions` | yes | yes | no | bulk session revoke, and single-session revoke by a non-owner (`RequireSelfOr` against the session owner) |
| `auth.view-user-sessions` | yes | yes | no | list a user's active sessions (`RequireSelfOr` against the target user) |

Dump every registered permission code as a JavaScript array (replaying the real
service registrations, not grepping) with:

```bash
make permissions
# or: go run ./cmd/cli permissions
```

## API reference

All application routes are mounted under the `/api` prefix. Authenticated
routes accept the access token from the `access_token` cookie or the
`Authorization: Bearer <token>` header. Errors are returned by the service; the
handler never writes an error body itself.

### Public routes

| Method | Path | Rate limit | Notes |
| --- | --- | --- | --- |
| `POST` | `/api/auth/login` | 5/min | Returns token pair and sets cookies |
| `POST` | `/api/auth/register` | — | Public, **unless** `APP_INTERNAL_MODE=true` (then authenticated + `auth.register-user`) |
| `POST` | `/api/auth/register/complete` | 5/min | Consumes invite token, creates the account |
| `GET` | `/api/auth/register/:token` | — | Inspect invite token status |
| `POST` | `/api/auth/refresh` | — | Rotates refresh token, returns a new pair |
| `POST` | `/api/auth/password/recover` | 5/min | Starts recovery; Redis per-email throttle 30 s |
| `GET` | `/api/auth/password/recover/:token` | — | Inspect reset token status |
| `PUT` | `/api/auth/password/reset` | — | Consumes reset token, sets a new password |
| `GET` | `/api/healthz` | — | Liveness + dependency (Postgres/Redis) check |

### Authenticated routes

| Method | Path | Required | Notes |
| --- | --- | --- | --- |
| `DELETE` | `/api/auth/logout` | session | Revokes the current session; always returns 200 |
| `GET` | `/api/auth/me` | self (`user.read`) | Current user |
| `PUT` | `/api/auth/password/change` | self or `auth.change-password` | Verifies current password; revokes all sessions |
| `GET` | `/api/auth/permissions` | authenticated | Lists the caller's permission codes (sorted) |
| `GET` | `/api/auth/sessions/users/:userID` | self or `auth.view-user-sessions` | List a user's active (unexpired, unrevoked) sessions |
| `DELETE` | `/api/auth/sessions/users/:userID` | `auth.revoke-user-sessions` | Bulk revoke all sessions/tokens for a user |
| `DELETE` | `/api/auth/sessions/:id` | session owner or `auth.revoke-user-sessions` | Revoke a single session |
| `GET` | `/api/users/` | `user.read` | List users (paginated, filterable, sortable) |
| `PUT` | `/api/users/:id` | self or `user.update` | Update `name`, `phone`, `gender` |
| `DELETE` | `/api/users/:id` | `user.delete` | Soft-delete; revokes sessions and access tokens |
| `PUT` | `/api/users/profile-picture` | self (`user.update`) | Multipart image upload (field `image`, ≤ 2 MB, jpeg/png/webp) |
| `DELETE` | `/api/users/profile-picture` | self (`user.update`) | Remove the caller's avatar |
| `GET` | `/api/audit/` | `audit.read` | List audit entries; `entity`, `actor` and date filters, paginated and sortable |

Notes:

- `/api/users/` is registered with a trailing slash.
- Static `/profile-picture` routes are declared before the `/:id` routes so they
  take precedence in Fiber's router.
- `PUT /api/users/:id` is self-service when the id is the caller's own; admin and
  system may update anyone. There is no separate "update my profile" path.
- `DELETE /api/users/:id` requires `user.delete`, so only admin/system can use it.
- Request bodies are limited to 5 MB globally (`MaxBodySize`).

### List query DSL

`GET /api/users/` accepts:

| Query param | Meaning | Notes |
| --- | --- | --- |
| `page` | 1-based page | `< 1` normalizes to `1` |
| `limit` | Page size | `< 1` → `DefaultLimit` (20); `> MaxLimit` → `MaxLimit` (100) |
| `sort` | Sort field | Allow-list: `id`, `created_at`, `updated_at`; unknown fields are dropped |
| `order` | `asc` / `desc` (case-insensitive) | Invalid/missing → `ASC`; only applied with a valid `sort` |
| `gender` | Filter by gender | Values validated against `M`/`F`/`O` (case-insensitive); unknown dropped |
| `role` | Filter by role | Values validated against `admin`/`user`/`system`; unknown dropped |

`EnableSplittingOnParsers` is on, so repeated/comma-separated values are
accepted for enum filters. Soft-deleted users are always excluded
(`deleted_at IS NULL`). The count query shares the filters and drops
`ORDER BY`/`LIMIT`.

The response `meta` echoes exactly what was applied (the filter, the
canonicalized sort, and the clamped pagination), so a client can render an
accurate "showing X of Y" state without duplicating validation:

```json
{
  "data": [ { "id": "…", "name": "Jane" } ],
  "meta": {
    "filter": { "role": ["admin"], "gender": ["F"] },
    "sort": { "by": "created_at", "order": "DESC" },
    "pagination": { "page": 1, "limit": 20, "total": 3, "total_pages": 1 }
  }
}
```

### Response envelope

Every successful response uses the sparse `Body` envelope (`internal/transport/http/body.go`),
passed by value:

```json
{ "data": {}, "message": "", "meta": {} }
```

- `data` and `message` use `omitempty`, `meta` uses `omitzero`; only populated
  fields are emitted and an empty envelope is `{}`.
- `data` is `any`, so `omitempty` drops it only while unset. A typed nil (nil
  slice/map/pointer) still marshals as `"data": null`.

### curl examples

Login (stores cookies in `cookies.txt`):

```bash
curl -sS -c cookies.txt -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"jane@example.com","password":"correct-horse-battery"}'
```

Authenticated call using the cookie jar (or `-H 'Authorization: Bearer <token>'`):

```bash
curl -sS -b cookies.txt http://localhost:8080/api/auth/me
```

List users with filters, sorting, and pagination:

```bash
curl -sS -b cookies.txt \
  'http://localhost:8080/api/users/?role=admin&gender=f&sort=created_at&order=desc&page=1&limit=20'
```

List a user's active sessions (its owner, or a role with
`auth.view-user-sessions`):

```bash
curl -sS -b cookies.txt \
  http://localhost:8080/api/auth/sessions/users/00000000-0000-0000-0000-000000000000
```

Revoke every session and access token for one user (admin/system):

```bash
curl -sS -b cookies.txt -X DELETE \
  http://localhost:8080/api/auth/sessions/users/00000000-0000-0000-0000-000000000000
```

Revoke a single session (its owner, or a role with
`auth.revoke-user-sessions`):

```bash
curl -sS -b cookies.txt -X DELETE \
  http://localhost:8080/api/auth/sessions/00000000-0000-0000-0000-000000000000
```

## Error handling

Services return typed errors from the `apperr` kernel. Handlers return them
unchanged; Fiber's `ErrorHandler` (`internal/transport/http/router.go`) calls
`ParseError`, which normalizes everything into:

```json
{ "error": { "message": "...", "details": null, "code": "..." } }
```

Status mapping:

| Source | Kind / case | HTTP status |
| --- | --- | --- |
| `apperr.KindValidation` | validation | 422 |
| `apperr.KindNotFound` | not found | 404 |
| `apperr.KindConflict` | conflict | 409 |
| `apperr.KindUnauthorized` | authentication missing/invalid | 401 |
| `apperr.KindForbidden` | authenticated but not permitted | 403 |
| `apperr.KindThrottled` | rate limited | 429 |
| `*fiber.Error` with code 413 | body too large | 413 |
| `context.DeadlineExceeded` | timeout | 504 |
| `context.Canceled` | client closed request | 499 |
| malformed JSON body | bind error | 400 |
| everything else | unmapped | 500 |

`apperr.Error` is immutable-returning (`SetMessage`/`WithDetails` produce
copies) and comparable with `errors.Is` by `Kind` + `Code`. Handler-level
sentinels (invalid path param, multipart, upload errors, rate limit) live in
`internal/transport/http/error.go` and `uploader.go`.

## Audit logging

`audit_logs` records user-initiated state changes for compliance and forensics.

- **Columns**: `actor_id` (NULL for system actions), `action`, `entity`,
  `entity_id`, `prev`/`next` JSONB snapshots, `meta` JSONB, `created_at`.
- **Actor/meta enrichment** (`Entry.FillActorMeta`, called by the repository at
  insert time): fills `actor_id` from the RBAC context and adds
  `request_id`, `ip_addr`, `user_agent`, `session_id`, and `actor_role` to
  `meta`. `ip_addr` comes from `AuditContext` middleware, which only honors
  forwarded headers from `TRUSTED_PROXIES`.
- **Atomicity**: audit rows are written inside the same transaction as the
  change they record, so a committed action is never silently unaudited.
- **Coverage** — actions declared beside their domain permissions:
  - auth: `auth.register`, `auth.complete-registration`, `auth.login`,
    `auth.logout`, `auth.change-password`, `auth.reset-password`,
    `auth.password-recovery-request`, `auth.revoke-user-sessions`,
    `auth.revoke-session`.
  - user: `user.update`, `user.delete`, `user.change-profile-picture`,
    `user.delete-profile-picture`.
- **Not audited**: refused attempts (failed login) and refresh-token reuse are
  security *events*, not state changes. They are emitted as structured WARN logs
  so the table stays clean and the unauthenticated login path takes no write.
- `GET /api/audit/` lists entries (requires `audit.read`). It accepts
  `entity`, `actor`, `date`/`from`/`to`, `sort`/`order` and `page`/`limit`, and
  reports the applied query in `meta`. `actor` matches `actor_id` exactly when it
  is a full UUID and otherwise matches the actor's name with `ILIKE`. A bare
  `date` is that UTC day; a full timestamp starts a 24-hour window at the instant
  sent, so a client can express its own day boundaries without the server knowing
  its timezone.

## Messaging and the worker

Email delivery is decoupled from HTTP response time using Redis Streams.

```text
HTTP request
  └─ service commits DB change
       └─ producer XADD ──▶ Stream ──▶ consumer group ──▶ PEL ──▶ handler ──▶ ACK
                                                              └─ retries ──▶ DLQ stream
```

Streams and payloads:

| Stream | Payload | Published by |
| --- | --- | --- |
| `email.password_recovery` | `auth.PasswordRecoveryMessage` | `RecoverPassword` |
| `email.user_registration` | `auth.CompleteRegistrationMessage` | `Register` |

The producer wraps each payload in a `messaging.Envelope[T]` carrying a random
`ID` (idempotency key) and a production timestamp, then `XADD`s the JSON under
the `payload` field. Consumers run with group `mailing` and consumer `c1m`,
concurrency 5, batch size 5, block 5 s, `MinIdle` 15 s, max retries 3, and DLQ
enabled (failed messages are written to `<stream>.dlq` with `_original_id`,
`_reason`, `_error`, `_failed_at`).

Reliability features (all in `internal/infrastructure/redis/stream_consumer.go`):

- **Idempotency**: before handling, `SET dedup:<stream>:<id> NX` with a 7-day
  TTL prevents duplicate emails on redelivery; the claim is released if handling
  fails so the retry re-processes.
- **Panic isolation**: a panic in a handler is recovered with a stack log, the
  dedup claim is released, and the message is routed to the DLQ — one poison
  message cannot crash the worker.
- **PEL and reclaim**: unacknowledged messages stay in the pending entries list;
  `XAUTOCLAIM` reclaims them on a 30 s cadence (not every poll) and the native
  delivery counter drives the retry/DLQ decision.
- **Bounded operations**: SMTP send is capped at 10 s (`pkg/mailer`), and
  ACK/DLQ writes use `context.WithoutCancel` so work completed during graceful
  shutdown is still acknowledged.
- **Graceful shutdown**: `cmd/worker` waits up to 10 s for consumers to drain.

Run the worker separately from the API: `make dev:worker`.

## Observability and health

- **Metrics** (`pkg/metrics`): `app_info` (labels `version`, `environment`),
  `app_request_duration` (labels `path`, `method`, `status_code`) and
  `app_request_total` (same labels). `path` uses the matched route template to
  keep label cardinality bounded. Instrumentation is a Fiber middleware that
  resolves the status from a returned error because Fiber assigns the final
  status only after the chain unwinds.
- **Exporter**: in production only, a separate Fiber app serves `/metrics` on
  `APP_PORT + 1`; the API itself listens on `127.0.0.1:APP_PORT`. The metrics
  sidecar is intentionally excluded from graceful shutdown.
- **Health**: `GET /api/healthz` pings Postgres and Redis with a 2 s timeout.
  Any failure returns **503** with a `dependencies` map of the failing services;
  success returns `{"status":"ok","internal_mode":<bool>}`. Because it doubles
  as a probe, a sustained dependency outage also fails liveness — split the
  endpoints if your platform restarts unhealthy instances.
- **Security headers**: Fiber `helmet` with a deny-by-default CSP, `X-Frame-Options: DENY`,
  a strict-origin-when-cross-origin referrer policy, and a restrictive
  permissions policy. HSTS is configured in production and emitted only on
  secure requests.
- **Graceful shutdown** (API): after `SIGINT`/`SIGTERM` the process waits 5 s to
  let load balancers drain, then shuts the server down with a 25 s deadline.
- **Dashboards**: Prometheus scrape config and Grafana provisioning/dashboards
  (`deployment/grafana/dashboards/api-metrics.json`) are checked into
  `deployment/` and wired by `compose.yml`.

## Database and migrations

- PostgreSQL, accessed through `pgx` (`pgxpool` for the pool) with **raw SQL**.
- `goose` migrations live in `migrations/` and are **additive only**:

  | Migration | Table |
  | --- | --- |
  | `00001_create_registration_tokens_table.sql` | `registration_tokens` |
  | `00002_create_users_table.sql` | `users` |
  | `00003_create_sessions_table.sql` | `sessions` |
  | `00004_create_password_recovery_tokens_table.sql` | `password_recovery_tokens` |
  | `00005_create_audit_logs_table.sql` | `audit_logs` |

- `users` has a partial unique index on `email` (`WHERE deleted_at IS NULL`) so a
  soft-deleted email can be reused; `role` and `gender` are `CHECK`-constrained.
- `sessions.user_id` and `audit_logs.actor_id` reference `users(id)` with
  `ON DELETE CASCADE` / `ON DELETE SET NULL` respectively.
- Transactions are implemented by `postgres.DB.Transact`: the `pgx.Tx` is stored
  in the context, and repositories fetch it with `db.GetConn(ctx)` (adding
  `FOR UPDATE` on transactional reads). Rollback/commit run on a
  `context.WithoutCancel` + 5 s timeout so a cancelled request context cannot
  leave a transaction (and its locks) hanging or churn the pool.

Create and apply migrations:

```bash
make migration:create   # prompts for a name, writes a goose SQL file
make migration:up       # apply all pending
make migration:status   # show current version
make migration:down     # roll back one
make migration:clear    # roll back all (down-to 0)
```

The `make migration:*` targets require the `goose` CLI and read `DB_*` from
`.env`.

## Testing

Unit tests only; they never touch a real database, Redis, or SMTP server.

- **Framework**: `testify` (`assert` for continuing checks, `require` for setup
  that must not continue) with table-driven `TestService_<Method>` +
  `t.Run("case", ...)` subtests.
- **Mocks**: generated by `mockery` from `.mockery.yml`.
  - Entity-scoped: `internal/auth/mocks/`, `internal/user/mocks/`.
  - Shared infrastructure: `internal/testing/mocks/` (`Transactor`, `Storage`,
    `File`, `Throttler`, `Recorder`, revocation `Checker`/`Revoker`/`Store`).
  - Regenerate after any interface change:

    ```bash
    mockery
    ```

- **Fixture pattern**: `setupTestFixture(t)` builds every mock with `t` (so an
  unexpected call fails the test), constructs the service under test, and
  registers `t.Cleanup` to run `AssertExpectations` on every mock. Transactor
  expectations are registered inside `RunAndReturn` because the inner
  expectations must exist before the closure runs.
- **Hermetic**: tests set what they need with `t.Setenv` (see
  `config/config_test.go`) and pass without a local `.env`.
- Run the suite:

  ```bash
  make test   # go test -race -v -count=1 ./... -cover
  ```

## Development tooling

| Target | Command | Purpose |
| --- | --- | --- |
| `make dev` | `air -c .air.toml` | API server with hot reload |
| `make dev:worker` | `air -c .air.worker.toml` | worker with hot reload |
| `make tidy` | `go mod tidy` | Tidy modules |
| `make lint` | `golangci-lint run` | Lint (v2 config in `.golangci.yml`) |
| `make test` | `go test -race -v -count=1 ./... -cover` | Full test suite |
| `make build` | `CGO_ENABLED=0 GOOS=linux go build …` | Static Linux binary at `./bin/api` |
| `make run` | `./bin/api` | Run the built binary |
| `make cli` | `go run ./cmd/cli $(ARGS)` | Developer CLI (currently `permissions`) |
| `make permissions` | `go run ./cmd/cli permissions` | Dump all permission codes as a JS array |
| `make migration:status` | `goose … status` | Migration status |
| `make migration:up` | `goose … up` | Apply migrations |
| `make migration:down` | `goose … down` | Roll back one migration |
| `make migration:clear` | `goose … down-to 0` | Roll back everything |
| `make migration:create` | `goose … create <name> sql` | Scaffold a migration |

`golangci-lint` must be **v2.x**; the config header recommends:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
```

## Conventions

- **Import aliases are fixed**: `redisInfra` for
  `internal/infrastructure/redis`, `strs` for `pkg/strings`, and `sharedMocks`
  for `internal/testing/mocks`. `internal/transport/http` is `package http` and
  is imported unaliased.
- **Naming**: packages are short, lowercase, single-word; files are
  `snake_case`; constructors are `New<Name>()`; sentinel errors use an `Err`
  prefix; constants are PascalCase.
- **Documentation**: every package has a package-level godoc comment and every
  exported symbol has a doc comment.
- **Interfaces are consumer-side and narrow**; implementations live in
  `internal/infrastructure/`.
- **Adding a feature** follows a fixed order: model + interface in
  `internal/<entity>/` → repository in `internal/infrastructure/postgres/` →
  service → handler → wire in `cmd/api/container.go` and routes in
  `cmd/api/server.go` → migration → update `.mockery.yml` + run `mockery` →
  tests.
- **Errors bubble up unchanged**; only add logging where it adds context, and
  avoid log-and-return pairs.
- **Multi-step writes** use `Transactor.Transact`; post-commit side effects
  (events, storage cleanup, access-token revocation) run outside the closure.

Project-specific skills under `.agents/skills/` (architecture, errors, handlers,
repositories, services, testing) encode these rules in more detail.

## License

MIT — see [`LICENSE`](LICENSE). Copyright (c) 2024 Adil Prawirdani.
