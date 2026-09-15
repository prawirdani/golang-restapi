package user

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/pkg/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		u, err := New("John Doe", "john@example.com", "hashedpassword")
		require.NoError(t, err)
		require.NotNil(t, u)

		assert.NotEqual(t, uuid.Nil, u.ID)
		assert.Equal(t, "John Doe", u.Name)
		assert.Equal(t, "john@example.com", u.Email)
		assert.Equal(t, "hashedpassword", u.Password)
		assert.Equal(t, rbac.RoleUser, u.Role)
		// Completing registration verifies the email at creation time.
		assert.True(t, u.EmailVerifiedAt.NotNull())
		// Optional profile fields start empty.
		assert.False(t, u.Phone.NotNull())
		assert.False(t, u.Gender.NotNull())
		assert.False(t, u.ProfilePicture.NotNull())
	})

	tests := []struct {
		title    string
		name     string
		email    string
		password string
	}{
		{"Empty name", "", "john@example.com", "hashedpassword"},
		{"Empty email", "John Doe", "", "hashedpassword"},
		{"Invalid email", "John Doe", "not-an-email", "hashedpassword"},
		{"Empty password", "John Doe", "john@example.com", ""},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			u, err := New(tt.name, tt.email, tt.password)

			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrValidation)
			assert.Nil(t, u)
		})
	}
}

func TestUser_Validate(t *testing.T) {
	tests := []struct {
		title       string
		user        User
		expectError bool
	}{
		{
			title: "Valid user with all fields",
			user: User{
				Name:     "John Doe",
				Email:    "john@example.com",
				Password: "hashedpassword",
				Role:     rbac.RoleUser,
				Gender:   nullable.New(GenderMale, true),
				Phone:    nullable.New("123456789", true),
			},
			expectError: false,
		},
		{
			title: "Valid user with optional fields null",
			user: User{
				Name:     "Jane Doe",
				Email:    "jane@example.com",
				Password: "hashedpassword",
				Role:     rbac.RoleUser,
			},
			expectError: false,
		},
		{
			title: "Empty name",
			user: User{
				Name:     "",
				Email:    "john@example.com",
				Password: "hashedpassword",
			},
			expectError: true,
		},
		{
			title: "Empty email",
			user: User{
				Name:     "John Doe",
				Email:    "",
				Password: "hashedpassword",
			},
			expectError: true,
		},
		{
			title: "Invalid email format",
			user: User{
				Name:     "John Doe",
				Email:    "not-an-email",
				Password: "hashedpassword",
			},
			expectError: true,
		},
		{
			title: "Empty password",
			user: User{
				Name:     "John Doe",
				Email:    "john@example.com",
				Password: "",
			},
			expectError: true,
		},
		{
			title: "Invalid gender",
			user: User{
				Name:     "John Doe",
				Email:    "john@example.com",
				Password: "hashedpassword",
				Gender:   nullable.New(Gender("X"), true),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			err := tt.user.Validate()
			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
