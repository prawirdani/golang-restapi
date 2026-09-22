package audit_test

import (
	"testing"

	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/stretchr/testify/assert"
)

func TestParseEntity(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		want   string
		wantOK bool
	}{
		{"user", "user", "user", true},
		{"session", "session", "session", true},
		{"registration token", "registration_token", "registration_token", true},
		{"canonicalises casing and padding", "  USER  ", "user", true},
		{"unknown name is rejected", "auth", "", false},
		{"a near miss is rejected", "users", "", false},
		{"empty is rejected", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := audit.ParseEntity(tt.value)

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
