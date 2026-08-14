package worker

import (
	"bytes"
	"context"

	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/pkg/mailer"
)

type AuthWorker struct {
	mailer *mailer.Mailer
}

func NewAuthWorker(mailer *mailer.Mailer) *AuthWorker {
	return &AuthWorker{
		mailer: mailer,
	}
}

// SendPasswordRecoveryEmail sends the password recovery email for a
// password recovery request.
//
// This method contains the email delivery logic and is transport-agnostic.
// It can be invoked by an event consumer, outbox worker, or other
// asynchronous delivery mechanism.
func (w *AuthWorker) SendPasswordRecoveryEmail(
	ctx context.Context,
	msg auth.PasswordRecoveryMessage,
) error {
	var buf bytes.Buffer
	if err := w.mailer.Templates.ResetPassword.Execute(&buf, map[string]any{
		"Name":    msg.Name,
		"Minutes": msg.Expiry.Minutes(),
		"URL":     msg.ResetURL,
	}); err != nil {
		return err
	}

	return w.mailer.Send(
		mailer.HeaderParams{
			To:      []string{msg.To},
			Subject: "Password Recovery",
		},
		buf,
	)
}
