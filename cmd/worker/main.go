package main

import (
	"context"
	"fmt"
	stdlog "log"
	"time"

	"github.com/prawirdani/golang-restapi/config"
	redisstream "github.com/prawirdani/golang-restapi/internal/infrastructure/messaging/redis"
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

	mail := mailer.New(cfg.SMTP)

	emailHandler := worker.NewEmailHandler(mail)

	emailEventConsumer := redisstream.NewStreamConsumer(rdb, redisstream.ConsumerConfig{
		Group:       "mailing",
		Stream:      "email.password_recovery",
		Consumer:    "c1",
		Concurrency: 5,
		BatchSize:   5,
		MaxRetries:  3,
		DLQStream:   "email.password_recovery.dlq",
		MinIdle:     time.Second * 15,
		Block:       time.Second * 5,
	}, emailHandler.HandlePasswordRecovery)

	if err := emailEventConsumer.Start(context.Background()); err != nil {
		stdlog.Fatal("Failed start consumer", err)
	}
}
