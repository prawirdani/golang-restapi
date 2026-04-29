package config

import (
	"os"
	"strconv"
)

type Redis struct {
	Host     string
	Port     int
	Password string
}

func (p *Redis) Parse() error {
	p.Host = os.Getenv("REDIS_HOST")
	p.Password = os.Getenv("REDIS_PASSWORD")

	if val := os.Getenv("REDIS_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			p.Port = port
		}
	}
	return nil
}
