package worker

import (
	"bytes"
	"context"

	"github.com/prawirdani/golang-restapi/internal/domain/auth"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/messaging"
	"github.com/prawirdani/golang-restapi/pkg/mailer"
)

type EmailHandler struct {
	mailer *mailer.Mailer
}

func NewEmailHandler(mailer *mailer.Mailer) *EmailHandler {
	return &EmailHandler{
		mailer: mailer,
	}
}

// HandlePasswordRecovery match [messaging.Handler] signature
func (w *EmailHandler) HandlePasswordRecovery(
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
