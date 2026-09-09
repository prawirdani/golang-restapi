package config

import (
	"fmt"
	"log"
	"net/url"

	"github.com/joho/godotenv"
)

type AppEnv string

const (
	EnvProduction  AppEnv = "prod"
	EnvDevelopment AppEnv = "dev"
)

type Config struct {
	App      App
	Postgres Postgres
	Redis    Redis
	Cors     Cors
	Auth     Auth
	SMTP     SMTP
	R2       R2
	Proxy    Proxy
}

func (c Config) IsProduction() bool {
	return c.App.Environment == EnvProduction
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load() // Load .env in dev

	cfg := &Config{}

	// Parse each struct
	if err := cfg.App.Parse(); err != nil {
		return nil, err
	}
	if err := cfg.Postgres.Parse(); err != nil {
		return nil, err
	}
	if err := cfg.Redis.Parse(); err != nil {
		return nil, err
	}
	if err := cfg.Cors.Parse(); err != nil {
		return nil, err
	}
	if err := cfg.Auth.Parse(); err != nil {
		return nil, err
	}
	if err := cfg.SMTP.Parse(); err != nil {
		return nil, err
	}
	if err := cfg.R2.Parse(); err != nil {
		return nil, err
	}
	if err := cfg.Proxy.Parse(); err != nil {
		return nil, err
	}

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.App.Environment != EnvProduction && c.App.Environment != EnvDevelopment {
		return fmt.Errorf("invalid APP_ENV, expecting %s or %s", EnvDevelopment, EnvProduction)
	}

	// JWT secret is the signing key for all tokens — a short or empty
	// secret is trivially brute-forceable. Fail startup rather than run insecure.
	if len(c.Auth.JwtSecret) < 32 {
		return fmt.Errorf("AUTH_JWT_SECRET is required and must be at least 32 characters")
	}

	// With CORS credentials enabled, a wildcard origin is invalid (browsers
	// reject it) and an unparseable origin would silently never match. Hard-fail
	// so misconfiguration surfaces at boot. Without credentials the CORS header
	// is harmless, so only warn.
	for _, origin := range c.Cors.Origins {
		if origin == "*" {
			if c.Cors.Credentials {
				return fmt.Errorf("CORS_CREDENTIALS=true is incompatible with wildcard origin %q", origin)
			}
			log.Printf("warning: wildcard CORS origin with credentials disabled: %s\n", origin)
			continue
		}
		if _, err := url.ParseRequestURI(origin); err != nil {
			if c.Cors.Credentials {
				return fmt.Errorf("invalid CORS origin %q: %w", origin, err)
			}
			log.Printf("warning: invalid CORS origin: %s\n", origin)
		}
	}

	return nil
}
