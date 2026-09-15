## Golang REST API
Personal Go RESTful API template with common 3 Layered architecture with following layers:
- `Handler/Controller/Delivery`: Responsible for handling incoming HTTP request, parsing request, validating request, calling service layer and sending response.
- `Service/Usecase`: Business logic layer, responsible for handling business logic, calling repository layer and returning response to handler.
- `Repository/Store`: Responsible for handling database operation, query, insert, update, delete etc.

--- 

### Folder Structure

```
cmd/
  api/                - HTTP server entrypoint (Fiber)
  worker/             - Message consumer/worker entrypoint

internal/
  auth/               - auth service, session, access token, password recovery
  user/               - user service and repository interfaces
  rbac/               - role/permission authorization (code-defined, in-memory)
  audit/              - audit recording (prev/next JSONB + request metadata)
  apperr/             - application error kernel: typed errors and kinds
  ports/              - interfaces implemented by infrastructure
    messaging/        -   message envelope + handler
    repository/       -   Transactor (atomic multi-repository writes)
    storage/          -   object storage (file, storage)
    throttle/         -   request throttling
  infrastructure/
    postgres/         - pgx repository implementations
    redis/            - Redis Streams producer/consumer, throttle
    r2/               - Cloudflare R2 storage
  transport/
    http/             - Fiber handlers, middleware, error normalization, router
  worker/             - Message consumer handler/worker

config/               - configuration structs (validated at startup)
pkg/                  - shared utilities (log, validator, mailer, metrics, nullable, strings)
migrations/           - database schema (goose)
```

### Configuration & Startup Validation

Config is loaded once from `.env` (see `.env.example`) and **validated at startup** — invalid values fail fast with a clear error instead of running insecurely:

- `AUTH_JWT_SECRET` — **required**, min 32 chars (HS256 key; rotating it forces all users to re-login)
- `DB_MAXCONNS` — required, must be > 0 (and `DB_MINCONNS` in `[0, DB_MAXCONNS]`)
- `CORS_CREDENTIALS=true` requires explicit valid `CORS_ORIGINS` (no `*`)
- `AUTH_PASSWORD_RECOVERY_TOKEN_TTL` — defaults to 5m (short window limits `?token=` URL exposure)
- `AUTH_REGISTRATION_TOKEN_TTL` — defaults to 15m (same reasoning for the registration link)
- `APP_INTERNAL_MODE` — when true, registration becomes admin-only (`auth.register-user` permission) instead of public
- `AUTH_COMPLETE_REGISTRATION_FORM_ENDPOINT` — web UI that hosts the password-creation form
- `TRUSTED_PROXIES` — comma-separated CIDRs/IPs of reverse proxies whose `X-Forwarded-For`/`X-Real-IP` are trusted for client IP resolution; when empty, only the direct peer IP is used (forwarded headers are ignored)

Run `make migration:up` after pulling — migrations are additive only.
--- 

### Authentication

The auth system uses a split-token design with JWT access tokens and opaque refresh tokens:

- **Access Token**: JWT signed with HS256, contains `sub` (user ID), `sid` (session ID) and `role`. Short TTL (default 60m via `AUTH_JWT_TTL`). Carried via cookie (`access_token`) or `Authorization: Bearer` header.

- **Refresh Token**: 256-bit cryptographically random opaque token, stored as SHA-256 hash in `sessions` table. Used to obtain new access/refresh token pairs.

- **Token Delivery**: Both tokens delivered as `HttpOnly` cookies (always; `Secure` in production). Access token has shorter TTL; refresh token persists for session lifetime (`AUTH_SESSION_TTL`, default 7d).

- **Refresh Token Rotation**: Every token refresh generates a new access token and rotates the refresh token. Old token hash is replaced with new hash in the same transaction, preventing replay attacks. Refresh attempts against a revoked session are logged and audited as a reuse signal.

- **Session Management**: Sessions are server-side records in `sessions` table with: `user_id`, `refresh_token_hash`, `ip_addr`, `user_agent`, `created_at`, `accessed_at`, `expires_at`, `revoked_at`. Refresh re-captures the client IP/user-agent. Sessions auto-expire (TTL) and can be manually revoked. **Password reset or change revokes all sessions for the user** — every device is logged out.

- **Rate Limiting**: Fiber in-process limiter (per IP) — 20 req/min globally in production, 5 req/min on `/api/auth/login` and `/api/auth/password/recover`. Password recovery additionally uses a Redis-backed per-email throttle (30s) so it holds across instances. Login verifies a dummy bcrypt hash on unknown emails to equalize timing (no account enumeration via login).

- **Passwords**: bcrypt cost 12; length validated 8–72 bytes (bcrypt truncates beyond 72). Reset tokens are 256-bit, single-use, and expire after a short TTL (default 5m, `AUTH_PASSWORD_RECOVERY_TOKEN_TTL`).

#### Registration (invitation)

Registration is invitation-based — no account exists until the invitee sets a password.

Flow: `POST /api/auth/register` (name + email) → rejects if the email is already registered → stores a single-use token (hashed, default 15m TTL via `AUTH_REGISTRATION_TOKEN_TTL`) → publishes to Redis Stream → worker emails the completion link → `POST /api/auth/register/complete` (token + password) creates the user and marks the token used, atomically.

- `GET /api/auth/register/:token` exposes token status (expiry / used) so the completion form can render it.
- When `APP_INTERNAL_MODE=true`, `POST /api/auth/register` requires the `auth.register-user` permission (admin/system only) instead of being public.
- The created user gets the default `user` role and has `email_verified_at` set (completing the invite proves the email).

#### Password Recovery

Flow: `POST /api/auth/password/recover` (submit email) → generates opaque token → stores hash in `password_recovery_tokens` (indexed by `token_hash`) → publishes msg to Redis Stream → worker sends HTML email with reset link. Token valid for 5m by default, single-use (marked as used after reset). Unknown email returns 404 to the client (enumeration-as-feature; kept deliberately).

**Message Queue**

Redis Stream with consumer groups. Each stream uses a consumer group with pending entries list (PEL) for reliable delivery and dead-letter handling:

```
stream → consumer group → pending entries (PEL) → ack → DLQ stream after MaxRetry
```

The worker consumes messages via `go-redis`. Auth publishes password-recovery (`email.password_recovery`) and registration-completion (`email.user_registration`) emails asynchronously to decouple SMTP from HTTP response time. Reliability features:

- **Idempotency**: each envelope carries an `ID`; a `SET ... NX` dedup key (`dedup:<stream>:<id>`) prevents duplicate emails on redelivery (at-least-once without duplicates).
- **Panic isolation**: message handler panics are recovered and routed to the DLQ — one poison message can't crash the worker.
- **Bounded operations**: SMTP send (10s) and worker shutdown (10s) are time-bounded; ack/DLQ writes use a `WithoutCancel` context so completed work is acked even during shutdown.
- **Reclaim throttled**: `XAUTOCLAIM` of pending messages runs on a 30s cadence, not every poll.

--- 

### Endpoints

Public:

| Method | Path | Notes |
| --- | --- | --- |
| POST | `/api/auth/register` | start invitation (admin-only when `APP_INTERNAL_MODE`) |
| POST | `/api/auth/register/complete` | set password, create account |
| GET | `/api/auth/register/:token` | inspect registration token |
| POST | `/api/auth/login` | 5 req/min per IP |
| POST | `/api/auth/refresh` | rotate refresh token |
| POST | `/api/auth/password/recover` | 5 req/min per IP |
| GET | `/api/auth/password/recover/:token` | inspect reset token |
| PUT | `/api/auth/password/reset` | consume reset token |
| GET | `/healthz` | liveness + dependency check |

Authenticated (cookie or `Authorization: Bearer`):

| Method | Path | Notes |
| --- | --- | --- |
| DELETE | `/api/auth/logout` | revoke current session |
| GET | `/api/auth/me` | current user |
| PUT | `/api/auth/password/change` | verify current password |
| PUT | `/api/users/` | update own profile |
| PUT | `/api/users/profile-picture` | multipart upload |
| DELETE | `/api/users/profile-picture` | remove avatar |

--- 

### Authorization (RBAC)

Role-based, code-defined and in-memory — no permission tables to keep in sync:

- Roles (`rbac.Role`): `admin`, `user`, `system` (background workers). The `users.role` column is constrained by a DB `CHECK` as a backstop.
- Permissions use the `"<entity>.<verb>"` grammar (e.g. `user.update`). Each entity declares its own role→permission table and registers it with the authorizer at startup.
- Services enforce with `Require(ctx, perms...)` (role must hold all) or `RequireSelfOr(ctx, userID, perm)` for ownership checks. The acting principal (user ID + role) is injected into the request context from the access token by the auth middleware.
- `admin` and `system` hold the user-management permissions; `user` reaches its own record through `RequireSelfOr`.

### Audit Logging

State-changing actions are recorded in `audit_logs`:

- **Payloads**: `prev` / `next` JSONB snapshots (password hashes are never serialized), plus a `meta` JSONB with `ip_addr`, `user_agent`, `request_id`, `session_id`, and `actor_role`.
- **Actor**: `actor_id` (NULL for system actions) and `actor_role`.
- **Coverage**: auth events (register, complete-registration, login, failed login, logout, password change/reset, recovery request, refresh-token reuse) and user mutations.
- **Atomicity**: success events are written inside the same transaction as the action; failure events (e.g. failed login) are best-effort and never block the response.

### Observability & Health

- **Metrics**: Prometheus request duration/count labelled by route template, method, and status, via a Fiber-native middleware. The exporter is served on `APP_PORT+1` in production; Grafana dashboards/provisioning live in `deployment/`.
- **Health**: `GET /healthz` pings Postgres and Redis and returns `503` with the failing dependencies when either is unreachable.
- **Security headers**: Fiber `helmet` (nosniff, frame deny, CSP, referrer/permissions policy); HSTS is configured in production and emitted on secure requests.
- **Client IP**: forwarded headers are honored only from `TRUSTED_PROXIES`, so the recorded IP can't be spoofed by direct clients.

--- 

### Technologies
- [PostgreSQL](https://www.postgresql.org)
- [Redis](https://redis.io) using stream as message queue
- [Cloudflare R2](https://www.cloudflare.com/developer-platform/products/r2)
- Metrics & Instrumentation:
  - [Prometheus](https://prometheus.io)
  - [Grafana](https://grafana.com)

#### Deps:
- Logger: std `log/slog` & [zerolog](https://github.com/rs/zerolog) (Swappable)
- Env loader: [godotenv](https://github.com/joho/godotenv)
- HTTP framework: [Fiber v3](https://github.com/gofiber/fiber)
- Postgres driver & pooling: [pgx](https://github.com/jackc/pgx)
- Postgres struct scanner: [scanny](https://github.com/georgysavva/scany)
- Unique Identifier: [uuid](https://github.com/google/uuid)
- Struct validator: [validator](https://github.com/go-playground/validator)
- SMTP Mailing: [gomail.v2](https://pkg.go.dev/gopkg.in/gomail.v2)
- Auth: [jwt](https://github.com/golang-jwt/jwt)
- Cloudflare R2 Client: [aws-sdk-v2](https://github.com/aws/aws-sdk-go-v2)
- Redis Client: [go-redis](https://github.com/redis/go-redis)
- Prometheus Client: [prometheus](https://github.com/prometheus/client_golang)
- Testing: [testify](https://github.com/stretchr/testify) & [mockery](https://github.com/vektra/mockery)

#### Tools:
  - Database Migration Tool: [goose](https://github.com/pressly/goose)
  - Development live reloading: [air](https://github.com/cosmtrek/air)
  - Linters: [golangci-lint](https://github.com/golangci/golangci-lint) **v2** (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`)


