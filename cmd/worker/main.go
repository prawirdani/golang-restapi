package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	stdlog "log"

	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/messaging/rabbitmq"
	"github.com/prawirdani/golang-restapi/internal/transport/amqp/consumer"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/prawirdani/golang-restapi/pkg/mailer"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		stdlog.Fatal("Failed to load config", err)
	}
	log.SetLogger(log.NewZerologAdapter(cfg.IsProduction()))

	rmqconn, err := initRabbitMQ(cfg.RabbitMQURL)
	if err != nil {
		log.Error("Failed to init rabbit mq", err)
		os.Exit(1)
	}
	defer rmqconn.Close()

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

	if err := startMessageConsumers(ctx, rmqconn, cfg); err != nil && err != context.Canceled {
		log.Error("Worker exited with error", err)
		cancel()
	}

	log.Info("Worker exited gracefully")
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

// Start message consumers, this function is blocking, so run it inside seperate goroutine
func startMessageConsumers(
	ctx context.Context,
	conn *amqp.Connection,
	cfg *config.Config,
) error {
	m := mailer.New(cfg.SMTP)
	authConsumers := consumer.NewAuthMessageConsumer(m)
	consumerClient := consumer.NewConsumerClient(conn)

	errCh := make(chan error, 1)

	// Run consumers in background
	// TODO: As things grows, consider using slice of [topology+handler] and run all of it through loops
	go func() {
		if err := consumerClient.Consume(
			ctx,
			rabbitmq.PasswordRecoveryEmailTopology,
			authConsumers.EmailPasswordRecoveryHandler,
		); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("consumer error: %w", err)
		}
		return nil
	}
}
