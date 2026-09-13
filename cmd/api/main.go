package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	stdlog "log"

	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/postgres"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/redis/go-redis/v9"
)

func init() {
	time.Local, _ = time.LoadLocation("UTC")
}

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		stdlog.Fatal("Failed to load config", err)
	}
	log.SetLogger(log.NewZerologAdapter(cfg.IsProduction()))

	pgpool, err := postgres.New(cfg.Postgres)
	if err != nil {
		log.Error("Failed to create postgres connection", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%v", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       0, // use default DB
	})

	container, err := NewContainer(cfg, pgpool, rdb)
	if err != nil {
		log.Error("Failed to create container", err)
		os.Exit(1)
	}

	server, err := NewServer(container, func(err error) error {
		pgpool.Close()
		if err := rdb.Close(); err != nil {
			log.Error("Failed to close redis conn", err)
			return err
		}

		return nil
	})
	if err != nil {
		log.Error("Failed to create server", err)
		os.Exit(1)
	}

	var isShuttingDown atomic.Bool
	// Start
	go func() {
		if err := server.Start(); err != nil {
			log.Error("Server listen error", err)
		}
	}()

	// Wait for signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Begin graceful shutdown
	isShuttingDown.Store(true)
	time.Sleep(5 * time.Second) // Let LB drain

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- server.Shutdown(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			log.Error("Server shutdown error", err)
			return
		}
		log.Info("Server shutdown gracefully")
	case <-ctx.Done():
		log.Error("Shutdown timed out", ctx.Err())
	}
}
