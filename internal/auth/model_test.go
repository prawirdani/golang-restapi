package auth

import (
	"testing"

	vld "github.com/prawirdani/golang-restapi/pkg/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginInput_Validate(t *testing.T) {
	tests := []struct {
		title    string
		input    LoginInput
		wantErr  bool
		errField string
	}{
		{
			title: "Valid input",
			input: LoginInput{
				Email:    "john@example.com",
				Password: "secret123",
			},
			wantErr: false,
		},
		{
			title: "Valid input with user agent",
			input: LoginInput{
				Email:     "john@example.com",
				Password:  "secret123",
				UserAgent: "Mozilla/5.0",
			},
			wantErr: false,
		},
		{
			title: "Empty email",
			input: LoginInput{
				Email:    "",
				Password: "secret123",
			},
			wantErr:  true,
			errField: "email",
		},
		{
			title: "Invalid email format",
			input: LoginInput{
				Email:    "not-an-email",
				Password: "secret123",
			},
			wantErr:  true,
			errField: "email",
		},
		{
			title: "Empty password",
			input: LoginInput{
				Email:    "john@example.com",
				Password: "",
			},
			wantErr:  true,
			errField: "password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			err := vld.Validate(&tt.input)
			if tt.wantErr {
				require.Error(t, err)
				var vErr *vld.ValidationError
				require.ErrorAs(t, err, &vErr)
				assert.True(t, vErr.HasField(tt.errField))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRecoverPasswordInput_Validate(t *testing.T) {
	tests := []struct {
		title    string
		input    RecoverPasswordInput
		wantErr  bool
		errField string
	}{
		{
			title:   "Valid email",
			input:   RecoverPasswordInput{Email: "john@example.com"},
			wantErr: false,
		},
		{
			title:    "Empty email",
			input:    RecoverPasswordInput{Email: ""},
			wantErr:  true,
			errField: "email",
		},
		{
			title:    "Invalid email format",
			input:    RecoverPasswordInput{Email: "not-an-email"},
			wantErr:  true,
			errField: "email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			err := vld.Validate(&tt.input)
			if tt.wantErr {
				require.Error(t, err)
				var vErr *vld.ValidationError
				require.ErrorAs(t, err, &vErr)
				assert.True(t, vErr.HasField(tt.errField))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestResetPasswordInput_Validate(t *testing.T) {
	tests := []struct {
		title    string
		input    ResetPasswordInput
		wantErr  bool
		errField string
	}{
		{
			title: "Valid input",
			input: ResetPasswordInput{
				Token:       "abc123",
				NewPassword: "newsecret123",
			},
			wantErr: false,
		},
		{
			title: "Empty token",
			input: ResetPasswordInput{
				Token:       "",
				NewPassword: "newsecret123",
			},
			wantErr:  true,
			errField: "token",
		},
		{
			title: "Empty new password",
			input: ResetPasswordInput{
				Token:       "abc123",
				NewPassword: "",
			},
			wantErr:  true,
			errField: "new_password",
		},
		{
			title: "New password too short",
			input: ResetPasswordInput{
				Token:       "abc123",
				NewPassword: "short",
			},
			wantErr:  true,
			errField: "new_password",
		},
		{
			title: "Password exactly 8 chars is valid",
			input: ResetPasswordInput{
				Token:       "abc123",
				NewPassword: "12345678",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			err := vld.Validate(&tt.input)
			if tt.wantErr {
				require.Error(t, err)
				var vErr *vld.ValidationError
				require.ErrorAs(t, err, &vErr)
				assert.True(t, vErr.HasField(tt.errField))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestChangePasswordInput_Validate(t *testing.T) {
	tests := []struct {
		title    string
		input    ChangePasswordInput
		wantErr  bool
		errField string
	}{
		{
			title: "Valid input",
			input: ChangePasswordInput{
				Password:    "oldpassword",
				NewPassword: "newpassword",
			},
			wantErr: false,
		},
		{
			title: "Empty password",
			input: ChangePasswordInput{
				Password:    "",
				NewPassword: "newpassword",
			},
			wantErr:  true,
			errField: "password",
		},
		{
			title: "Empty new password",
			input: ChangePasswordInput{
				Password:    "oldpassword",
				NewPassword: "",
			},
			wantErr:  true,
			errField: "new_password",
		},
		{
			title: "New password too short",
			input: ChangePasswordInput{
				Password:    "oldpassword",
				NewPassword: "short",
			},
			wantErr:  true,
			errField: "new_password",
		},
		{
			title: "Both fields empty",
			input: ChangePasswordInput{
				Password:    "",
				NewPassword: "",
			},
			wantErr:  true,
			errField: "password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			err := vld.Validate(&tt.input)
			if tt.wantErr {
				require.Error(t, err)
				var vErr *vld.ValidationError
				require.ErrorAs(t, err, &vErr)
				assert.True(t, vErr.HasField(tt.errField))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
