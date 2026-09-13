package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

type Postgres struct {
	User            string
	Password        string
	Host            string
	Port            int
	Name            string
	MinConns        int
	MaxConns        int
	MaxConnLifetime time.Duration
}

func (p *Postgres) Parse() error {
	p.User = os.Getenv("DB_USER")
	p.Password = os.Getenv("DB_PASSWORD")
	p.Host = os.Getenv("DB_HOST")
	p.Name = os.Getenv("DB_NAME")

	if val := os.Getenv("DB_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			p.Port = port
		}
	}
	if val := os.Getenv("DB_MINCONNS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			p.MinConns = i
		}
	}
	if val := os.Getenv("DB_MAXCONNS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			p.MaxConns = i
		}
	}
	if val := os.Getenv("DB_MAXCONN_LIFETIME"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			p.MaxConnLifetime = d
		}
	}

	// pgxpool treats MaxConns=0 as "no limit", but the intended default when
	// DB_MAXCONNS is unset is 0 here, and a 0 value passed through the pool
	// config silently unbounds the pool. Fail with a clear message instead of a
	// cryptic boot error.
	if p.MaxConns <= 0 {
		return fmt.Errorf("DB_MAXCONNS must be set to a value > 0 (got %d)", p.MaxConns)
	}
	// pgxpool stores these as int32; reject values that would truncate on
	// conversion instead of silently wrapping to a negative pool size.
	if p.MaxConns > math.MaxInt32 {
		return fmt.Errorf("DB_MAXCONNS must be <= %d (got %d)", math.MaxInt32, p.MaxConns)
	}
	if p.MinConns < 0 || p.MinConns > p.MaxConns {
		return fmt.Errorf("DB_MINCONNS must be between 0 and DB_MAXCONNS (got %d)", p.MinConns)
	}
	return nil
}
