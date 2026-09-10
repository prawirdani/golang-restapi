-- +goose Up
-- +goose StatementBegin
SELECT
  'up SQL query';

CREATE TABLE IF NOT EXISTS sessions (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  refresh_token_hash BYTEA NOT NULL UNIQUE,
  user_agent VARCHAR(255) NOT NULL,
  ip_addr INET NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now (),
  accessed_at TIMESTAMPTZ NOT NULL DEFAULT now (),
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_sessions_user_id ON sessions (user_id);

CREATE INDEX idx_sessions_revoked_at ON sessions (revoked_at);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
SELECT
  'down SQL query';

DROP TABLE IF EXISTS sessions;

-- +goose StatementEnd
