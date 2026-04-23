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
  worker/             - AMQP consumer entrypoint

internal/
  domain/             - business models, services, interfaces
    auth/             - auth service, session, access token, password recovery
    user/             - user service and repository interfaces
  infrastructure/
    repository/       - postgres implementations
    messaging/        - rabbitmq publisher & consumer
    storage/          - r2 implementation
  transport/
    http/             - chi handlers, middleware
    amqp/             - consumer implementations

config/               - configuration structs
pkg/                  - shared utilities (log, validator, mailer, metrics)
migrations/           - database schema (goose)
```
--- 

### Authentication

The auth system uses a split-token design with JWT access tokens and opaque refresh tokens:

- **Access Token**: JWT signed with HS256, contains `sub` (user ID) and `sid` (session ID). Short TTL (default 15min). Carried via cookie (`access_token`) or `Authorization: Bearer` header.

- **Refresh Token**: 256-bit cryptographically random opaque token, stored as SHA-256 hash in `sessions` table. Used to obtain new access/refresh token pairs.

- **Token Delivery**: Both tokens delivered as `HttpOnly` cookies in production. Access token has shorter TTL; refresh token persists for session lifetime.

- **Refresh Token Rotation**: Every token refresh generates a new access token and rotates the refresh token. Old token hash is replaced with new hash in the same transaction, preventing replay attacks.

- **Session Management**: Sessions are server-side records in `sessions` table with: `user_id`, `refresh_token` (hashed), `user_agent`, `expires_at`, `accessed_at`, `revoked_at`. Sessions auto-expire and can be manually revoked.

#### Password Recovery

Flow: `/auth/recover` (submit email) → generates opaque token → stores hash in `password_recovery_tokens` → publishes msg to RabbitMQ → worker sends HTML email with reset link. Token valid for configured TTL (configurable via .env), single-use (marked as used after reset).

**Message Queue**

RabbitMQ with direct exchange topology per message type. Each queue has built-in retry (dlx) and dead-letter (dlq) handling:

```
main queue → retry queue (TTL) → back to main → DLQ after MaxRetry
```

The worker consumes messages via `amqp091-go`. Auth domain publishes password recovery emails asynchronously to decouple SMTP from HTTP response time.

--- 

### Technologies
- [PostgreSQL](https://www.postgresql.org)
- [RabbitMQ](https://www.rabbitmq.com)
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
- RabbitMQ Client: [amqp091-go](https://github.com/rabbitmq/amqp091-go)
- Prometheus Client: [prometheus](https://github.com/prometheus/client_golang)
- Testing: [testify](https://github.com/stretchr/testify) & [mockery](https://github.com/vektra/mockery)

#### Tools:
  - Database Migration Tool: [goose](https://github.com/pressly/goose)
  - Development live reloading: [air](https://github.com/cosmtrek/air)
  - Linters: [golangci-lint](https://github.com/golangci/golangci-lint)


