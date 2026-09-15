package config

import (
	"os"
	"strconv"
	"strings"
)

type App struct {
	Name        string
	Version     string
	Port        int
	Environment AppEnv
	// InternalMode makes user registration admin-only (APP_INTERNAL_MODE)
	// instead of public self-service.
	InternalMode bool
}

func (a *App) Parse() error {
	a.Name = os.Getenv("APP_NAME")
	a.Version = os.Getenv("APP_VERSION")
	a.Environment = AppEnv(strings.ToLower(os.Getenv("APP_ENV")))

	if val := os.Getenv("APP_PORT"); val != "" {
		port, err := strconv.Atoi(val)
		if err != nil {
			return err
		}
		a.Port = port
	}

	if val := os.Getenv("APP_INTERNAL_MODE"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			a.InternalMode = b
		}
	}

	return nil
}
