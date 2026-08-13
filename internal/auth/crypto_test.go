package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword(t *testing.T) {
	plain := "my_password"

	hashed, err := HashPassword(plain)
	assert.Nil(t, err)
	assert.NotEqual(t, plain, string(hashed))
}

func TestVerifyPassword(t *testing.T) {
	plain := "my_password"
	hashed, err := HashPassword(plain)
	assert.Nil(t, err)

	t.Run("success", func(t *testing.T) {
		err := VerifyPassword(plain, string(hashed))
		require.Nil(t, err)
	})

	t.Run("wrong-password", func(t *testing.T) {
		err := VerifyPassword(plain, "wrong-password")
		require.NotNil(t, err)
		require.Equal(t, err, ErrWrongCredentials)
	})
}
