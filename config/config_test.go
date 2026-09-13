package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// setRequiredEnv sets the minimum env LoadConfig needs to succeed.
func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "dev")
	t.Setenv("AUTH_JWT_SECRET", "test-secret-at-least-32-characters-long")
	t.Setenv("DB_MAXCONNS", "5")
}

func TestLoadConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		setRequiredEnv(t)

		cfg, err := LoadConfig()
		require.NoError(t, err)
		require.NotNil(t, cfg)
		require.Equal(t, EnvDevelopment, cfg.App.Environment)
		require.Equal(t, 5, cfg.Postgres.MaxConns)
	})

	t.Run("fails on short jwt secret", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("AUTH_JWT_SECRET", "too-short")

		_, err := LoadConfig()
		require.Error(t, err)
	})

	t.Run("fails without max conns", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("DB_MAXCONNS", "0")

		_, err := LoadConfig()
		require.Error(t, err)
	})

	t.Run("fails on invalid env", func(t *testing.T) {
		setRequiredEnv(t)
		t.Setenv("APP_ENV", "staging")

		_, err := LoadConfig()
		require.Error(t, err)
	})
}
