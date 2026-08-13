## Golang REST API
Personal Go RESTful API template with common 3 Layered architecture with following layers:
- `Handler/Controller/Delivery`: Responsible for handling incoming HTTP request, parsing request, validating request, calling service layer and sending response.
- `Service/Usecase`: Business logic layer, responsible for handling business logic, calling repository layer and returning response to handler.
- `Repository/Store`: Responsible for handling database operation, query, insert, update, delete etc.

--- 

### Folder Structure

```
cmd/
  api/                - HTTP server entrypoint
  worker/             - Message consumer/worker entrypoint

internal/
  auth/               - auth service, session, access token, password recovery
  user/               - user service and repository interfaces
  apperr/             - application error kernel: typed errors and kinds
  infrastructure/
    repository/       - postgres implementations
    messaging/        - redis stream producer & consumer implementations
    storage/          - r2 implementation
  transport/
    http/             - chi handlers, middleware
  worker/             - Message consumer handler/worker

config/               - configuration structs (validated at startup)
pkg/                  - shared utilities (log, validator, mailer, metrics)
migrations/           - database schema (goose)
```

### Configuration & Startup Validation

Config is loaded once from `.env` (see `.env.example`) and **validated at startup** — invalid values fail fast with a clear error instead of running insecurely:

- `AUTH_JWT_SECRET` — **required**, min 32 chars (HS256 key; rotating it forces all users to re-login)
- `DB_MAXCONNS` — required, must be > 0
- `CORS_CREDENTIALS=true` requires explicit valid `CORS_ORIGINS` (no `*`)
- `AUTH_PASSWORD_RECOVERY_TOKEN_TTL` — defaults to 5m (short window limits `?token=` URL exposure)

Run `make migration:up` after pulling — migrations are additive only.
--- 

### Authentication

The auth system uses a split-token design with JWT access tokens and opaque refresh tokens:

- **Access Token**: JWT signed with HS256, contains `sub` (user ID) and `sid` (session ID). Short TTL (default 60m via `AUTH_JWT_TTL`). Carried via cookie (`access_token`) or `Authorization: Bearer` header.

- **Refresh Token**: 256-bit cryptographically random opaque token, stored as SHA-256 hash in `sessions` table. Used to obtain new access/refresh token pairs.

- **Token Delivery**: Both tokens delivered as `HttpOnly` cookies (always; `Secure` in production). Access token has shorter TTL; refresh token persists for session lifetime (`AUTH_SESSION_TTL`, default 7d).

- **Refresh Token Rotation**: Every token refresh generates a new access token and rotates the refresh token. Old token hash is replaced with new hash in the same transaction, preventing replay attacks. Refresh attempts against a revoked session are logged as a reuse signal.

- **Session Management**: Sessions are server-side records in `sessions` table with: `user_id`, `refresh_token` (hashed), `user_agent`, `expires_at`, `accessed_at`, `revoked_at`. Sessions auto-expire and can be manually revoked. Expired sessions are pruned on login. **Password reset or change revokes all sessions for the user** — every device is logged out.

- **Rate Limiting**: per-IP throttle via Redis. Global 50 req/min in production; `/api/auth/login` and `/api/auth/password/recover` limited to 5 req/min each. Login verifies a dummy bcrypt hash on unknown emails to equalize timing (no account enumeration via login).

- **Passwords**: bcrypt cost 12; length validated 8–72 bytes (bcrypt truncates beyond 72). Reset tokens are 256-bit, single-use, and expire after a short TTL (default 5m, `AUTH_PASSWORD_RECOVERY_TOKEN_TTL`).

#### Password Recovery

Flow: `POST /api/auth/password/recover` (submit email) → generates opaque token → stores hash in `password_recovery_tokens` (indexed by `token_hash`) → publishes msg to Redis Stream → worker sends HTML email with reset link. Token valid for 5m by default, single-use (marked as used after reset). Unknown email returns 404 to the client (enumeration-as-feature; kept deliberately).

**Message Queue**

Redis Stream with consumer groups. Each stream uses a consumer group with pending entries list (PEL) for reliable delivery and dead-letter handling:

```
stream → consumer group → pending entries (PEL) → ack → DLQ stream after MaxRetry
```

The worker consumes messages via `go-redis`. Auth publishes password recovery emails asynchronously to decouple SMTP from HTTP response time. Reliability features:

- **Idempotency**: each envelope carries an `ID`; a `SETNX` dedup key prevents duplicate emails on redelivery (at-least-once without duplicates).
- **Panic isolation**: message handler panics are recovered and routed to the DLQ — one poison message can't crash the worker.
- **Bounded operations**: SMTP send (10s) and worker shutdown (10s) are time-bounded; ack/DLQ writes use a `WithoutCancel` context so completed work is acked even during shutdown.
- **Reclaim throttled**: `XAUTOCLAIM` of pending messages runs on a 30s cadence, not every poll.

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
- ENV Loader: [godotenv](https://github.com/joho/godotenv)
- HTTP router: [chi](https://github.com/go-chi/chi)
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
  - Linters: [golangci-lint](https://github.com/golangci/golangci-lint)


