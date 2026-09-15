package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prawirdani/golang-restapi/config"
	redisInfra "github.com/prawirdani/golang-restapi/internal/infrastructure/redis"
	"github.com/prawirdani/golang-restapi/internal/worker"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/prawirdani/golang-restapi/pkg/mailer"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

// shutdownTimeout bounds how long main waits for Start to return after cancel
// (ctx) so a wedged handler can't block deferred cleanup (rdb.Close) forever.
const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		log.Error("Application stopped", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log.SetLogger(log.NewZerologAdapter(cfg.IsProduction()))

	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       0,
	})
	defer rdb.Close()

	// Verify Redis connectivity before starting consumers.
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}

	// Dependencies.
	m := mailer.New(cfg.SMTP)
	authWorker := worker.NewAuthWorker(m)

	authEventConsumers := redisInfra.NewAuthEventConsumers(rdb, authWorker)

	// Register all consumers here.
	consumers := []redisInfra.Consumer{
		authEventConsumers.PasswordRecovery,
		authEventConsumers.CompleteRegistration,
		// Add more consumers as the application grows:
		// authEvents.EmailVerification,
		// notificationEvents.PushNotification,
		// billingEvents.Payment,
	}

	return runConsumers(ctx, consumers)
}

func runConsumers(
	ctx context.Context,
	consumers []redisInfra.Consumer,
) error {
	if len(consumers) == 0 {
		return nil
	}

	g, ctx := errgroup.WithContext(ctx)

	for _, consumer := range consumers {
		g.Go(func() error {
			err := consumer.Start(ctx)

			if err != nil && !errors.Is(err, context.Canceled) {
				return err
			}

			return nil
		})
	}

	err := waitForShutdown(ctx, g)
	if err != nil {
		return err
	}

	return nil
}

func waitForShutdown(
	ctx context.Context,
	g *errgroup.Group,
) error {
	done := make(chan error, 1)

	go func() {
		done <- g.Wait()
	}()

	select {
	case err := <-done:
		// A consumer stopped before shutdown.
		if err != nil {
			return fmt.Errorf("consumer stopped: %w", err)
		}

		return nil

	case <-ctx.Done():
		log.Info("Shutdown signal received")

		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			shutdownTimeout,
		)
		defer cancel()

		// The errgroup is using the signal context, so all consumers
		// should receive cancellation and begin graceful shutdown.
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf(
					"consumer stopped during shutdown: %w",
					err,
				)
			}

			log.Info("All consumers exited gracefully")
			return nil

		case <-shutdownCtx.Done():
			return fmt.Errorf(
				"consumer shutdown timed out after %s",
				shutdownTimeout,
			)
		}
	}
}
