package redis

import (
	"context"

	"github.com/prawirdani/golang-restapi/internal/domain/auth"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/messaging"
	"github.com/redis/go-redis/v9"
)

type EmailProducer struct {
	rdb *redis.Client
}

func NewEmailProducer(rdb *redis.Client) *EmailProducer {
	return &EmailProducer{rdb: rdb}
}

// PasswordRecovery implements [auth.Mailer]
func (p *EmailProducer) PasswordRecovery(ctx context.Context, msg auth.PasswordRecoveryMessage) error {
	return produce(ctx, p.rdb, "email.password_recovery", messaging.NewEnvelope(msg))
}
