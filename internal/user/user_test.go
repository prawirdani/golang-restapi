package user

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/pkg/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		title        string
		input        CreateUserInput
		expectError  error
		validateUser func(*testing.T, *User)
	}{
		{
			title: "Success with phone",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "john@example.com",
				Phone:    "123456789",
				Password: "hashedpassword",
			},
			expectError: nil,
			validateUser: func(t *testing.T, user *User) {
				assert.NotEqual(t, uuid.Nil, user.ID)
				assert.Equal(t, "John Doe", user.Name)
				assert.Equal(t, "john@example.com", user.Email)
				assert.Equal(t, "hashedpassword", user.Password)
				assert.True(t, user.Phone.NotNull())
				assert.Equal(t, "123456789", user.Phone.Get())
				assert.False(t, user.ProfilePicture.NotNull())
			},
		},
		{
			title: "Success without phone",
			input: CreateUserInput{
				Name:     "Jane Doe",
				Email:    "jane@example.com",
				Phone:    "",
				Password: "hashedpassword",
			},
			expectError: nil,
			validateUser: func(t *testing.T, user *User) {
				assert.NotEqual(t, uuid.Nil, user.ID)
				assert.Equal(t, "Jane Doe", user.Name)
				assert.Equal(t, "jane@example.com", user.Email)
				assert.Equal(t, "hashedpassword", user.Password)
				assert.False(t, user.Phone.NotNull())
				assert.Equal(t, "", user.Phone.Get())
				assert.False(t, user.ProfilePicture.NotNull())
			},
		},
		{
			title: "Success with gender",
			input: CreateUserInput{
				Name:     "Jane Doe",
				Email:    "jane@example.com",
				Phone:    "123456789",
				Gender:   "M",
				Password: "hashedpassword",
			},
			expectError: nil,
			validateUser: func(t *testing.T, user *User) {
				assert.NotEqual(t, uuid.Nil, user.ID)
				assert.Equal(t, "Jane Doe", user.Name)
				assert.Equal(t, "jane@example.com", user.Email)
				assert.Equal(t, "hashedpassword", user.Password)
				assert.True(t, user.Phone.NotNull())
				assert.Equal(t, "123456789", user.Phone.Get())
				assert.True(t, user.Gender.NotNull())
				assert.Equal(t, GenderMale, user.Gender.Get())
				assert.False(t, user.ProfilePicture.NotNull())
			},
		},
		{
			title: "Validation error empty name",
			input: CreateUserInput{
				Name:     "",
				Email:    "john@example.com",
				Phone:    "123456789",
				Password: "hashedpassword",
			},
			expectError:  ErrValidation,
			validateUser: nil,
		},
		{
			title: "Validation error empty email",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "",
				Phone:    "123456789",
				Password: "hashedpassword",
			},
			expectError:  ErrValidation,
			validateUser: nil,
		},
		{
			title: "Validation error empty password",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "john@example.com",
				Phone:    "123456789",
				Password: "",
			},
			expectError:  ErrValidation,
			validateUser: nil,
		},
		{
			title: "Profile picture is null by default",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "john@example.com",
				Phone:    "123456789",
				Password: "hashedpassword",
			},
			expectError: nil,
			validateUser: func(t *testing.T, user *User) {
				assert.False(t, user.ProfilePicture.NotNull())
				assert.Equal(t, "", user.ProfilePicture.Get())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			inp := tt.input
			user, err := New(inp.Name, inp.Email, inp.Phone, Gender(inp.Gender), inp.Password)

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
