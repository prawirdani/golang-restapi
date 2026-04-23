package consumer

import (
	"bytes"
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/prawirdani/golang-restapi/internal/domain/auth"
	"github.com/prawirdani/golang-restapi/pkg/mailer"
)

type AuthMessageConsumer struct {
	mailer *mailer.Mailer
}

func NewAuthMessageConsumer(mailer *mailer.Mailer) *AuthMessageConsumer {
	return &AuthMessageConsumer{
		mailer: mailer,
	}
}

func (mc *AuthMessageConsumer) EmailPasswordRecoveryHandler(
	ctx context.Context,
	d amqp.Delivery,
) error {
	msg, err := decodeJsonBody[auth.PasswordRecoveryEmailMessage](d.Body)
	if err != nil {
		return fmt.Errorf("failed to decode body: %w", err)
	}

	// Execute template
	var buf bytes.Buffer
	if err := mc.mailer.Templates.ResetPassword.Execute(&buf, map[string]any{
		"Name":    msg.Name,
		"Minutes": msg.Expiry.Minutes(),
		"URL":     msg.ResetURL,
	}); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	if err := mc.mailer.Send(
		mailer.HeaderParams{To: []string{msg.To}, Subject: "Password Recovery"},
		buf,
	); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}
