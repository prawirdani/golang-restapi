// Package user provides the data model and business logic for managing users in system.
package user

import (
	"net/mail"
	"time"

	"github.com/google/uuid"

	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/pkg/nullable"
)

var (
	ErrEmailConflict = apperr.ConflictErr("email already exists", "USER_EMAIL_CONFLICT")
	ErrValidation    = apperr.ValidationErr("invalid user data", "USER_VALIDATION")
)

type User struct {
	ID              uuid.UUID                    `db:"id"                json:"id"`
	Name            string                       `db:"name"              json:"name"`
	Email           string                       `db:"email"             json:"email"`
	EmailVerifiedAt nullable.Nullable[time.Time] `db:"email_verified_at" json:"email_verified_at"`
	Password        string                       `db:"password"          json:"-"`
	Gender          nullable.Nullable[Gender]    `db:"gender"            json:"gender"`
	Phone           nullable.Nullable[string]    `db:"phone"             json:"phone"`
	Role            rbac.Role                    `db:"role"              json:"role"`
	ProfilePicture  nullable.Nullable[string]    `db:"profile_picture"   json:"profile_picture"`
	CreatedAt       time.Time                    `db:"created_at"        json:"created_at"`
	UpdatedAt       time.Time                    `db:"updated_at"        json:"updated_at"`
}

func (u *User) Validate() error {
	if u.Name == "" {
		return ErrValidation.WithDetails("name is required")
	}
	if u.Email == "" {
		return ErrValidation.WithDetails("email is required")
	}
	if _, err := mail.ParseAddress(u.Email); err != nil {
		return ErrValidation.WithDetails("email must be a valid email address")
	}
	if u.Password == "" {
		return ErrValidation.WithDetails("password is required")
	}

	if u.Gender.NotNull() && !u.Gender.Get().IsValid() {
		return ErrValidation.WithDetails("invalid gender")
	}

	if !u.Role.Valid() {
		return ErrValidation.WithDetails("invalid role")
	}

	return nil
}

// New builds a user from a completed registration: default role, email
// verified, optional profile fields (phone/gender/picture) left empty.
// Returns an error if validation fails.
func New(name, email string, hashedPassword string) (*User, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	u := User{
		ID:       id,
		Name:     name,
		Email:    email,
		Password: hashedPassword,
		Role:     rbac.RoleUser,
	}
	u.EmailVerifiedAt.Set(time.Now(), false)

	if err := u.Validate(); err != nil {
		return nil, err
	}

	return &u, nil
}
