package redis

import (
	"context"
	"time"

	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/ports/messaging"
	"github.com/prawirdani/golang-restapi/internal/worker"
	"github.com/redis/go-redis/v9"
)

const (
	// EmailPasswordRecoveryStream is the Redis stream carrying password-recovery email events.
	EmailPasswordRecoveryStream = "email.password_recovery"
)

// AuthEventProducer publishes auth-related events (e.g. password recovery) to Redis streams.
// It implements [auth.EventProducer].
type AuthEventProducer struct {
	rdb *redis.Client
}

// NewAuthEventProducer constructs an AuthEventProducer backed by the given Redis client.
func NewAuthEventProducer(rdb *redis.Client) *AuthEventProducer {
	return &AuthEventProducer{
		rdb: rdb,
	}
}

// ProducePasswordRecoveryEvent implements [auth.EventProducer].
func (ep *AuthEventProducer) ProducePasswordRecoveryEvent(ctx context.Context, msg auth.PasswordRecoveryMessage) error {
	return produceStream(
		ctx,
		ep.rdb,
		EmailPasswordRecoveryStream,
		messaging.NewEnvelope(msg),
	)
}

// AuthEventConsumers groups the stream consumers that process auth-related events.
type AuthEventConsumers struct {
	PasswordRecovery Consumer
}

// NewAuthEventConsumers wires the auth event stream consumers to the given worker.
func NewAuthEventConsumers(
	rdb *redis.Client,
	wrk *worker.AuthWorker,
) *AuthEventConsumers {
	return &AuthEventConsumers{
		PasswordRecovery: NewStreamConsumer(
			rdb,
			ConsumerStreamConfig{
				Group:       "mailing",
				Stream:      EmailPasswordRecoveryStream,
				Consumer:    "c1m",
				Concurrency: 5,
				BatchSize:   5,
				MaxRetries:  3,
				UseDLQ:      true,
				MinIdle:     15 * time.Second,
				Block:       5 * time.Second,
			},
			func(
				ctx context.Context,
				env messaging.Envelope[auth.PasswordRecoveryMessage],
			) error {
				return wrk.SendPasswordRecoveryEmail(
					ctx,
					env.Payload,
				)
			},
		),
	}
}
