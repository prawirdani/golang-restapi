package user

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name          string
		nameInput     string
		emailInput    string
		phoneInput    string
		passwordInput string
		expectError   error
		validateUser  func(*testing.T, *User)
	}{
		{
			name:          "Success with phone",
			nameInput:     "John Doe",
			emailInput:    "john@example.com",
			phoneInput:    "123456789",
			passwordInput: "hashedpassword",
			expectError:   nil,
			validateUser: func(t *testing.T, user *User) {
				assert.NotEqual(t, uuid.Nil, user.ID)
				assert.Equal(t, "John Doe", user.Name)
				assert.Equal(t, "john@example.com", user.Email)
				assert.Equal(t, "hashedpassword", user.Password)
				assert.True(t, user.Phone.Valid())
				assert.Equal(t, "123456789", user.Phone.Get())
				assert.False(t, user.ProfileImage.Valid())
			},
		},
		{
			name:          "Success without phone",
			nameInput:     "Jane Doe",
			emailInput:    "jane@example.com",
			phoneInput:    "",
			passwordInput: "hashedpassword",
			expectError:   nil,
			validateUser: func(t *testing.T, user *User) {
				assert.NotEqual(t, uuid.Nil, user.ID)
				assert.Equal(t, "Jane Doe", user.Name)
				assert.Equal(t, "jane@example.com", user.Email)
				assert.Equal(t, "hashedpassword", user.Password)
				assert.False(t, user.Phone.Valid())
				assert.Equal(t, "", user.Phone.Get())
				assert.False(t, user.ProfileImage.Valid())
			},
		},
		{
			name:          "Validation error empty name",
			nameInput:     "",
			emailInput:    "john@example.com",
			phoneInput:    "123456789",
			passwordInput: "hashedpassword",
			expectError:   ErrValidation,
			validateUser:  nil,
		},
		{
			name:          "Validation error empty email",
			nameInput:     "John Doe",
			emailInput:    "",
			phoneInput:    "123456789",
			passwordInput: "hashedpassword",
			expectError:   ErrValidation,
			validateUser:  nil,
		},
		{
			name:          "Validation error empty password",
			nameInput:     "John Doe",
			emailInput:    "john@example.com",
			phoneInput:    "123456789",
			passwordInput: "",
			expectError:   ErrValidation,
			validateUser:  nil,
		},
		{
			name:          "Profile image is null by default",
			nameInput:     "John Doe",
			emailInput:    "john@example.com",
			phoneInput:    "123456789",
			passwordInput: "hashedpassword",
			expectError:   nil,
			validateUser: func(t *testing.T, user *User) {
				assert.False(t, user.ProfileImage.Valid())
				assert.Equal(t, "", user.ProfileImage.Get())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := New(tt.nameInput, tt.emailInput, tt.phoneInput, tt.passwordInput)

			if tt.expectError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectError)
				assert.Nil(t, user)
			} else {
				require.NoError(t, err)
				require.NotNil(t, user)
				if tt.validateUser != nil {
					tt.validateUser(t, user)
				}
			}
		})
	}
}
