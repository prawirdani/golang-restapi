package redis

import (
	"context"

	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/messaging"
	"github.com/redis/go-redis/v9"
)

const (
	EmailPasswordRecoveryStream = "email.password_recovery"
)

type EmailEventProducer struct {
	rdb *redis.Client
}

func NewEmailEventProducer(rdb *redis.Client) *EmailEventProducer {
	return &EmailEventProducer{rdb: rdb}
}

// PasswordRecovery implements [auth.Mailer]
func (p *EmailEventProducer) PasswordRecovery(ctx context.Context, msg auth.PasswordRecoveryMessage) error {
	return produce(ctx, p.rdb, EmailPasswordRecoveryStream, messaging.NewEnvelope(msg))
}
