package config

import (
	"os"
	"time"
)

type Auth struct {
	JwtSecret                        string
	JwtTTL                           time.Duration
	SessionTTL                       time.Duration
	PasswordRecoveryTokenTTL         time.Duration
	RegistrationTokenTTL             time.Duration
	ResetPasswordFormEndpoint        string
	CompleteRegistrationFormEndpoint string
}

func (t *Auth) Parse() error {
	t.JwtSecret = os.Getenv("AUTH_JWT_SECRET")
	t.ResetPasswordFormEndpoint = os.Getenv("AUTH_RESET_PASSWORD_FORM_ENDPOINT")
	t.CompleteRegistrationFormEndpoint = os.Getenv("AUTH_COMPLETE_REGISTRATION_FORM_ENDPOINT")

	if val := os.Getenv("AUTH_JWT_TTL"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			t.JwtTTL = d
		}
	}
	if val := os.Getenv("AUTH_SESSION_TTL"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			t.SessionTTL = d
		}
	}
	if val := os.Getenv("AUTH_PASSWORD_RECOVERY_TOKEN_TTL"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			t.PasswordRecoveryTokenTTL = d
		}
	}
	if val := os.Getenv("AUTH_REGISTRATION_TOKEN_TTL"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			t.RegistrationTokenTTL = d
		}
	}
	// Default to a short TTL (5m) even when the env var is unset. The reset
	// link travels in a URL (?token=) which can end up in logs/proxies/history,
	// so a short window limits the exposure. Env var overrides this default.
	if t.PasswordRecoveryTokenTTL == 0 {
		t.PasswordRecoveryTokenTTL = 5 * time.Minute
	}
	// Registration links travel in a URL the same way; default to a short window.
	if t.RegistrationTokenTTL == 0 {
		t.RegistrationTokenTTL = 15 * time.Minute
	}
	return nil
}
