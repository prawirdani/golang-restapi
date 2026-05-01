package main

import (
	"context"
	"fmt"
	stdlog "log"
	"os"
	"os/signal"
	"syscall"

	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/worker"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/prawirdani/golang-restapi/pkg/mailer"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		stdlog.Fatal("Failed to load config", err)
	}
	log.SetLogger(log.NewZerologAdapter(cfg.IsProduction()))

	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%v", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       0, // use default DB
	})
	defer rdb.Close()

	mailer := mailer.New(cfg.SMTP)
	emailEventConsumer := worker.NewEmailEventConsumer(mailer).Handler(rdb)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		cancel()
	}()

	go func() {
		if err := emailEventConsumer.Start(ctx); err != nil {
			stdlog.Fatal(err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
}
