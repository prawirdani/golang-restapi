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
	"github.com/prawirdani/golang-restapi/internal/infrastructure/messaging/rabbitmq"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/repository/postgres"
	"github.com/prawirdani/golang-restapi/pkg/log"
	amqp "github.com/rabbitmq/amqp091-go"
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

	rmqconn, err := initRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Error("Failed to init rabbit mq", err)
		os.Exit(1)
	}
	defer rmqconn.Close()

	container, err := NewContainer(cfg, pgpool, rmqconn)
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

func initRabbitMQ(url string) (*amqp.Connection, error) {
	conn, err := rabbitmq.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	if err := rabbitmq.SetupTopologies(
		conn,
		rabbitmq.PasswordRecoveryEmailTopology,
	); err != nil {
		return nil, fmt.Errorf("setup topologies: %w", err)
	}

	return conn, nil
}
