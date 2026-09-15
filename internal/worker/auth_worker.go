package worker

import (
	"bytes"
	"context"
	"fmt"

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

// SendCompleteRegistrationEmail renders and sends the "complete your
// registration" email carrying the password-creation link.
func (w *AuthWorker) SendCompleteRegistrationEmail(
	ctx context.Context,
	msg auth.CompleteRegistrationMessage,
) error {
	var buf bytes.Buffer
	// The template expects {{.Expiry}} as a pre-formatted, human-readable
	// string (it does no duration math itself).
	if err := w.mailer.Templates.CompleteRegistration.Execute(&buf, map[string]any{
		"Name":   msg.Name,
		"Expiry": fmt.Sprintf("%d minutes", int(msg.Expiry.Minutes())),
		"URL":    msg.URL,
	}); err != nil {
		return err
	}

	return w.mailer.Send(
		mailer.HeaderParams{
			To:      []string{msg.To},
			Subject: "Complete Registration",
		},
		buf,
	)
}
