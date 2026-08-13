package user

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGender_IsValid(t *testing.T) {
	tests := []struct {
		title   string
		gender  Gender
		isValid bool
	}{
		{title: "Male", gender: GenderMale, isValid: true},
		{title: "Female", gender: GenderFemale, isValid: true},
		{title: "Other", gender: GenderOther, isValid: true},
		{title: "Lowercase m", gender: "m", isValid: false},
		{title: "Lowercase f", gender: "f", isValid: false},
		{title: "Empty string", gender: "", isValid: false},
		{title: "Random string", gender: "invalid", isValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			assert.Equal(t, tt.isValid, tt.gender.IsValid())
		})
	}
}
