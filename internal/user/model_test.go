package user

import (
	"testing"

	vld "github.com/prawirdani/golang-restapi/pkg/validator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateUserInput_Validate(t *testing.T) {
	tests := []struct {
		title      string
		input      CreateUserInput
		wantErr    bool
		errField   string
	}{
		{
			title: "Valid input",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "john@example.com",
				Password: "secret1234",
			},
			wantErr: false,
		},
		{
			title: "Valid input with all fields",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "john@example.com",
				Phone:    "123456789",
				Password: "secret1234",
				Gender:   "M",
			},
			wantErr: false,
		},
		{
			title: "Empty name",
			input: CreateUserInput{
				Name:     "",
				Email:    "john@example.com",
				Password: "secret1234",
			},
			wantErr:  true,
			errField: "name",
		},
		{
			title: "Empty email",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "",
				Password: "secret1234",
			},
			wantErr:  true,
			errField: "email",
		},
		{
			title: "Invalid email format",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "not-an-email",
				Password: "secret1234",
			},
			wantErr:  true,
			errField: "email",
		},
		{
			title: "Empty password",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "john@example.com",
				Password: "",
			},
			wantErr:  true,
			errField: "password",
		},
		{
			title: "Password too short",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "john@example.com",
				Password: "short",
			},
			wantErr:  true,
			errField: "password",
		},
		{
			title: "Invalid gender",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "john@example.com",
				Password: "secret1234",
				Gender:   "X",
			},
			wantErr:  true,
			errField: "gender",
		},
		{
			title: "Valid lowercase gender",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "john@example.com",
				Password: "secret1234",
				Gender:   "m",
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

func TestCreateUserInput_Sanitize(t *testing.T) {
	tests := []struct {
		title    string
		input    CreateUserInput
		wantName string
		wantMail string
		wantPh   string
		wantGen  string
	}{
		{
			title: "Trims extra spaces",
			input: CreateUserInput{
				Name:     "  John   Doe  ",
				Email:    "  john@example.com  ",
				Phone:    "  123456789  ",
				Gender:   " M ",
				Password: "secret1234",
			},
			wantName: "John Doe",
			wantMail: "john@example.com",
			wantPh:   "123456789",
			wantGen:  "M",
		},
		{
			title: "No extra spaces",
			input: CreateUserInput{
				Name:     "John Doe",
				Email:    "john@example.com",
				Phone:    "123456789",
				Gender:   "F",
				Password: "secret1234",
			},
			wantName: "John Doe",
			wantMail: "john@example.com",
			wantPh:   "123456789",
			wantGen:  "F",
		},
		{
			title: "Empty fields stay empty",
			input: CreateUserInput{
				Name:     "",
				Email:    "",
				Phone:    "",
				Gender:   "",
				Password: "secret1234",
			},
			wantName: "",
			wantMail: "",
			wantPh:   "",
			wantGen:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			err := tt.input.Sanitize()
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, tt.input.Name)
			assert.Equal(t, tt.wantMail, tt.input.Email)
			assert.Equal(t, tt.wantPh, tt.input.Phone)
			assert.Equal(t, tt.wantGen, tt.input.Gender)
		})
	}
}

func TestUpdateUserInput_Validate(t *testing.T) {
	tests := []struct {
		title    string
		input    UpdateUserInput
		wantErr  bool
		errField string
	}{
		{
			title: "Valid input",
			input: UpdateUserInput{
				Name: "John Doe",
			},
			wantErr: false,
		},
		{
			title: "Valid input with all fields",
			input: UpdateUserInput{
				Name:   "John Doe",
				Phone:  "123456789",
				Gender: "F",
			},
			wantErr: false,
		},
		{
			title: "Empty name",
			input: UpdateUserInput{
				Name: "",
			},
			wantErr:  true,
			errField: "name",
		},
		{
			title: "Invalid gender",
			input: UpdateUserInput{
				Name:   "John Doe",
				Gender: "invalid",
			},
			wantErr:  true,
			errField: "gender",
		},
		{
			title: "Valid lowercase gender",
			input: UpdateUserInput{
				Name:   "John Doe",
				Gender: "o",
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

func TestUpdateUserInput_Sanitize(t *testing.T) {
	tests := []struct {
		title    string
		input    UpdateUserInput
		wantName string
		wantPh   string
		wantGen  string
	}{
		{
			title: "Trims extra spaces",
			input: UpdateUserInput{
				Name:   "  John   Doe  ",
				Phone:  "  123456789  ",
				Gender: " F ",
			},
			wantName: "John Doe",
			wantPh:   "123456789",
			wantGen:  "F",
		},
		{
			title: "No extra spaces",
			input: UpdateUserInput{
				Name:   "John Doe",
				Phone:  "123456789",
				Gender: "M",
			},
			wantName: "John Doe",
			wantPh:   "123456789",
			wantGen:  "M",
		},
		{
			title: "Empty fields stay empty",
			input: UpdateUserInput{
				Name:   "",
				Phone:  "",
				Gender: "",
			},
			wantName: "",
			wantPh:   "",
			wantGen:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			err := tt.input.Sanitize()
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, tt.input.Name)
			assert.Equal(t, tt.wantPh, tt.input.Phone)
			assert.Equal(t, tt.wantGen, tt.input.Gender)
		})
	}
}
