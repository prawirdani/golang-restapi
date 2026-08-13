package apperr_test

import (
	"errors"
	"testing"

	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/stretchr/testify/assert"
)

func TestError_Error(t *testing.T) {
	t.Run("Without details", func(t *testing.T) {
		err := apperr.ErrNotFound
		assert.Equal(t, "the requested resource was not found", err.Error())
	})

	t.Run("With string details", func(t *testing.T) {
		err := apperr.ErrNotFound.WithDetails("user 123")
		assert.Contains(t, err.Error(), "the requested resource was not found")
		assert.Contains(t, err.Error(), "details:")
	})

	t.Run("With complex details", func(t *testing.T) {
		err := apperr.ErrNotFound.WithDetails(map[string]string{"key": "value"})
		assert.Contains(t, err.Error(), "details:")
	})
}

func TestError_Is(t *testing.T) {
	t.Run("Same kind and code", func(t *testing.T) {
		err := apperr.ValidationErr("msg", "CODE_A")
		target := apperr.ValidationErr("other", "CODE_A")
		assert.True(t, errors.Is(err, target))
	})

	t.Run("Different kind", func(t *testing.T) {
		err := apperr.ValidationErr("msg", "CODE_A")
		target := apperr.ConflictErr("msg", "CODE_A")
		assert.False(t, errors.Is(err, target))
	})

	t.Run("Different code", func(t *testing.T) {
		err := apperr.ValidationErr("msg", "CODE_A")
		target := apperr.ValidationErr("msg", "CODE_B")
		assert.False(t, errors.Is(err, target))
	})

	t.Run("Non-AppError target", func(t *testing.T) {
		err := apperr.ErrNotFound
		target := errors.New("not an app error")
		assert.False(t, errors.Is(target, err))
	})
}

func TestError_Code(t *testing.T) {
	err := apperr.ValidationErr("msg", "MY_CODE")
	assert.Equal(t, "MY_CODE", err.Code())
}

func TestError_Kind(t *testing.T) {
	err := apperr.ConflictErr("msg", "CODE")
	assert.Equal(t, apperr.KindConflict, err.Kind())
}

func TestError_WithDetails(t *testing.T) {
	t.Run("With non-nil details", func(t *testing.T) {
		err := apperr.ErrNotFound
		updated := err.WithDetails("extra info")
		assert.Equal(t, "extra info", updated.Details)
		assert.Nil(t, err.Details) // original unchanged
	})

	t.Run("With nil details preserves original", func(t *testing.T) {
		err := apperr.ErrNotFound.WithDetails("original")
		updated := err.WithDetails(nil)
		assert.Equal(t, "original", updated.Details)
	})
}

func TestError_SetMessage(t *testing.T) {
	err := apperr.ErrNotFound
	updated := err.SetMessage("custom message")
	assert.Equal(t, "custom message", updated.Message)
	assert.Equal(t, "the requested resource was not found", err.Message) // original unchanged
}

func TestConstructErr(t *testing.T) {
	t.Run("Default code", func(t *testing.T) {
		err := apperr.UnauthorizedErr("msg")
		assert.Equal(t, "UNAUTHORIZED", err.Code())
		assert.Equal(t, apperr.KindUnauthorized, err.Kind())
	})

	t.Run("Custom code", func(t *testing.T) {
		err := apperr.UnauthorizedErr("msg", "CUSTOM_CODE")
		assert.Equal(t, "CUSTOM_CODE", err.Code())
	})

	t.Run("Empty string code uses default", func(t *testing.T) {
		err := apperr.ForbiddenErr("msg", "")
		assert.Equal(t, "FORBIDDEN", err.Code())
	})
}
