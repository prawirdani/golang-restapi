package worker

import (
	"bytes"
	"context"
	"time"

	"github.com/prawirdani/golang-restapi/internal/domain/auth"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/messaging"
	redisstream "github.com/prawirdani/golang-restapi/internal/infrastructure/messaging/redis"
	"github.com/prawirdani/golang-restapi/pkg/mailer"
	"github.com/redis/go-redis/v9"
)

type EmailEventConsumer struct {
	mailer *mailer.Mailer
}

func NewEmailEventConsumer(mailer *mailer.Mailer) *EmailEventConsumer {
	return &EmailEventConsumer{
		mailer: mailer,
	}
}

// HandlePasswordRecovery match [messaging.Handler] signature
func (w *EmailEventConsumer) HandlePasswordRecovery(
	ctx context.Context,
	envelope messaging.Envelope[auth.PasswordRecoveryMessage],
) error {
	var buf bytes.Buffer
	if err := w.mailer.Templates.ResetPassword.Execute(&buf, map[string]any{
		"Name":    envelope.Payload.Name,
		"Minutes": envelope.Payload.Expiry.Minutes(),
		"URL":     envelope.Payload.ResetURL,
	}); err != nil {
		return err
	}

	return w.mailer.Send(
		mailer.HeaderParams{
			To:      []string{envelope.Payload.To},
			Subject: "Password Recovery",
		},
		buf,
	)
}

func (h *EmailEventConsumer) Handler(rdb *redis.Client) redisstream.Consumer {
	return redisstream.NewStreamConsumer(
		rdb,
		redisstream.ConsumerConfig{
			Group:       "mailing",
			Stream:      redisstream.EmailPasswordRecoveryStream,
			Consumer:    "c1m",
			Concurrency: 5,
			BatchSize:   5,
			MaxRetries:  3,
			UseDLQ:      true,
			MinIdle:     time.Second * 15,
			Block:       time.Second * 5,
		},
		h.HandlePasswordRecovery,
	)
}
