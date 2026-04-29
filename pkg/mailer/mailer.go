package mailer

import (
	"bytes"
	"fmt"

	"github.com/prawirdani/golang-restapi/config"
	"gopkg.in/gomail.v2"
)

type HeaderParams struct {
	To      []string
	Cc      []string
	Subject string
}

type Mailer struct {
	Templates  *Templates
	dialer     *gomail.Dialer
	senderName string
}

func New(cfg config.SMTP) *Mailer {
	dialer := gomail.NewDialer(
		cfg.Host,
		cfg.Port,
		cfg.AuthEmail,
		cfg.AuthPassword,
	)

	templates := parseTemplates()

	return &Mailer{
		dialer:     dialer,
		Templates:  templates,
		senderName: cfg.SenderName,
	}
}

func (m *Mailer) Send(headerParams HeaderParams, body bytes.Buffer) error {
	mail := m.createHeader(headerParams)
	mail.SetBody("text/html", body.String())

	if err := m.dialer.DialAndSend(mail); err != nil {
		return fmt.Errorf("failed to send mail: %w", err)
	}

	return nil
}

func (m *Mailer) createHeader(params HeaderParams) *gomail.Message {
	mail := gomail.NewMessage()

	mail.SetHeader("From", m.senderName)
	mail.SetHeader("To", params.To...)
	mail.SetHeader("Cc", params.Cc...)
	mail.SetHeader("Subject", params.Subject)

	return mail
}
