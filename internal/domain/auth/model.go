// Package auth provides authentication and authorization functionality.
// This package handles user authentication through sessions, access tokens, and
// password management including secure hashing and password reset flows. It manages
// the complete authentication lifecycle from login through logout, including token
// generation, validation, and session management.
package auth

import (
	"time"

	"github.com/prawirdani/golang-restapi/pkg/strings"
)

type RegisterInput struct {
	Name     string `json:"name"     validate:"required"`
	Email    string `json:"email"    validate:"required,email"`
	Phone    string `json:"phone"`
	Password string `json:"password" validate:"required,min=8"`
}

// Sanitize implements [validator.Sanitizer]
func (r *RegisterInput) Sanitize() error {
	r.Email = strings.TrimSpacesConcat(r.Email)
	r.Name = strings.TrimSpacesConcat(r.Name)
	r.Phone = strings.TrimSpacesConcat(r.Phone)
	return nil
}

type LoginInput struct {
	Email     string `json:"email"    validate:"required,email"`
	Password  string `json:"password" validate:"required"`
	UserAgent string
}

type RecoverPasswordInput struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordInput struct {
	Token       string `json:"token"        validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

type ChangePasswordInput struct {
	Password    string `json:"password"     validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// Messaging/queue payload shape for PasswordRecovery job
type PasswordRecoveryMessage struct {
	To       string        `json:"to"`         // Recipient's email address
	Name     string        `json:"name"`       // Recipient's name
	ResetURL string        `json:"reset_url"`  // Link for resetting the password
	Expiry   time.Duration `json:"expiry_min"` // Expiration time of the reset token in minutes
}
