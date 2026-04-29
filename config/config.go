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
	for _, origin := range c.Cors.Origins {
		if _, err := url.ParseRequestURI(origin); err != nil {
			log.Printf("warning: invalid CORS origin: %s\n", origin)
		}
	}
	return nil
}
