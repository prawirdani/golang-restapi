package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	stdlog "log"

	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/repository/postgres"
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
	defer pgpool.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%v", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       0, // use default DB
	})
	defer rdb.Close()

	container, err := NewContainer(cfg, pgpool, rdb)
	if err != nil {
		log.Error("Failed to create container", err)
		os.Exit(1)
	}

	server, err := NewServer(container)
	if err != nil {
		log.Error("Failed to create server", err)
		os.Exit(1)
	}

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Capture OS signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		cancel()
	}()

	// Run HTTP server
	if err := server.Start(ctx); err != nil {
		log.Error("Server exited with error", err)
	}

	log.Info("Application exited gracefully")
}
