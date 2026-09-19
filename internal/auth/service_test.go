package auth_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/auth/mocks"
	"github.com/prawirdani/golang-restapi/internal/ports/throttle"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	sharedMocks "github.com/prawirdani/golang-restapi/internal/testing/mocks"
	"github.com/prawirdani/golang-restapi/internal/user"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

func init() {
	log.SetLogger(log.EmptyLog)
}

// auditCtx returns a context carrying audit request metadata, required by
// NewSession/Rotate to stamp the session's IP and user-agent.
func auditCtx() context.Context {
	return audit.WithContext(context.Background(), audit.Context{
		IP:        net.ParseIP("203.0.113.5"),
		UserAgent: "test-agent",
		RequestID: "test-request",
	})
}

func TestService_Register(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.RegisterInput{
			Name:  "John Doe",
			Email: "john@example.com",
		}

		// No existing user -> issues and stores a registration token, then
		// enqueues the completion email.
		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(nil, apperr.ErrNotFound)
				// A new invitation supersedes any outstanding one.
				f.authRepo.EXPECT().RevokeRegistrationTokens(ctx, input.Email).Return(nil)
				f.authRepo.EXPECT().
					StoreRegistrationToken(ctx, mock.AnythingOfType("*auth.RegistrationToken")).
					Return(nil)
				return fn(ctx)
			})

		f.eventProducer.EXPECT().
			ProduceRegistrationCompletionEvent(ctx, mock.AnythingOfType("auth.CompleteRegistrationMessage")).
			Return(nil)

		err := f.service.Register(ctx, input)

		assert.NoError(t, err)
	})

	t.Run("Email already exists", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.RegisterInput{
			Name:  "John Doe",
			Email: "john@example.com",
		}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.userRepo.EXPECT().
					GetByEmail(ctx, input.Email).
					Return(&user.User{ID: uuid.New(), Email: input.Email}, nil)
				return fn(ctx)
			})

		err := f.service.Register(ctx, input)

		assert.Error(t, err)
		assert.ErrorIs(t, err, user.ErrEmailConflict)
	})
}

func TestService_CompleteRegistration(t *testing.T) {
	newToken := func(t *testing.T) *auth.RegistrationToken {
		t.Helper()
		token, _, err := auth.NewRegistrationToken("John Doe", "john@example.com", time.Hour)
		require.NoError(t, err)
		return token
	}

	// expectGetToken wires the transact + token lookup for the non-success paths.
	expectGetToken := func(f *testFixture, ctx context.Context, token *auth.RegistrationToken, err error) {
		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.authRepo.EXPECT().
					GetRegistrationToken(ctx, mock.AnythingOfType("[]uint8")).
					Return(token, err)
				return fn(ctx)
			})
	}

	validInput := auth.CompleteRegistrationInput{Token: "regt_raw", Password: "newpassword123"}

	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)
		token := newToken(t)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.authRepo.EXPECT().
					GetRegistrationToken(ctx, mock.AnythingOfType("[]uint8")).
					Return(token, nil)
				f.authRepo.EXPECT().UpdateRegistrationToken(ctx, token).Return(nil)
				f.userRepo.EXPECT().Store(ctx, mock.AnythingOfType("*user.User")).Return(nil)
				return fn(ctx)
			})

		err := f.service.CompleteRegistration(ctx, validInput)

		require.NoError(t, err)
		assert.True(t, token.UsedAt.NotNull())
	})

	t.Run("Unknown token", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)
		expectGetToken(f, ctx, nil, apperr.ErrNotFound)

		err := f.service.CompleteRegistration(ctx, validInput)

		assert.ErrorIs(t, err, auth.ErrInvalidRegistrationToken)
	})

	t.Run("Already used", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)
		token := newToken(t)
		token.Use() // consume it
		expectGetToken(f, ctx, token, nil)

		err := f.service.CompleteRegistration(ctx, validInput)

		// A consumed token is rejected like any other invalid token.
		assert.ErrorIs(t, err, auth.ErrInvalidRegistrationToken)
	})

	t.Run("Revoked token", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)
		token := newToken(t)
		token.Revoke()
		expectGetToken(f, ctx, token, nil)

		err := f.service.CompleteRegistration(ctx, validInput)

		assert.ErrorIs(t, err, auth.ErrInvalidRegistrationToken)
	})

	t.Run("Expired token", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)
		token := newToken(t)
		token.ExpiresAt = time.Now().Add(-time.Hour)
		expectGetToken(f, ctx, token, nil)

		err := f.service.CompleteRegistration(ctx, validInput)

		assert.ErrorIs(t, err, auth.ErrInvalidRegistrationToken)
	})
}

func TestService_Login(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := auditCtx()
		f := setupTestFixture(t)

		input := auth.LoginInput{
			Email:    "john@example.com",
			Password: "password123",
		}

		hashedPassword, err := auth.HashPassword(input.Password)
		require.NoError(t, err)

		mockUser := &user.User{
			ID:       uuid.New(),
			Name:     "John Doe",
			Email:    input.Email,
			Password: string(hashedPassword),
		}

		f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(mockUser, nil)
		// Session store and its audit record commit in one transaction.
		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.authRepo.EXPECT().StoreSession(ctx, mock.AnythingOfType("*auth.Session")).Return(nil)
				return fn(ctx)
			})

		tokenPair, err := f.service.Login(ctx, input)

		assert.NoError(t, err)
		assert.NotNil(t, tokenPair)
	})

	t.Run("User not found", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.LoginInput{
			Email:    "nonexistent@example.com",
			Password: "password123",
		}

		f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(nil, apperr.ErrNotFound)

		tokenPair, err := f.service.Login(ctx, input)

		assert.Error(t, err)
		assert.ErrorIs(t, err, auth.ErrWrongCredentials)
		assert.Nil(t, tokenPair)
	})

	t.Run("Wrong password", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.LoginInput{
			Email:    "john@example.com",
			Password: "wrongpassword",
		}

		hashedPassword, err := auth.HashPassword("password123")
		require.NoError(t, err)

		mockUser := &user.User{
			ID:       uuid.New(),
			Name:     "John Doe",
			Email:    input.Email,
			Password: string(hashedPassword),
		}

		f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(mockUser, nil)

		tokenPair, err := f.service.Login(ctx, input)

		assert.Error(t, err)
		assert.ErrorIs(t, err, auth.ErrWrongCredentials)
		assert.Nil(t, tokenPair)
	})
}

func TestService_RefreshAccessToken(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := auditCtx()
		f := setupTestFixture(t)

		session, refreshToken, err := auth.NewSession(ctx, uuid.New(), f.cfg.SessionTTL)
		require.NoError(t, err)

		prevRefreshToken := refreshToken

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.authRepo.EXPECT().
					GetSessionByRefreshTokenHash(ctx, mock.AnythingOfType("[]uint8")).
					Return(session, nil)
				f.userRepo.EXPECT().
					GetByID(ctx, session.UserID).
					Return(&user.User{ID: session.UserID, Role: rbac.RoleUser}, nil)
				f.authRepo.EXPECT().UpdateSession(ctx, session).Return(nil)

				return fn(ctx)
			})

		tokenPair, err := f.service.RefreshAccessToken(ctx, prevRefreshToken)

		assert.NoError(t, err)
		assert.NotNil(t, tokenPair)
		assert.NotEqual(t, prevRefreshToken, tokenPair.RefreshToken)
	})

	t.Run("Session expired", func(t *testing.T) {
		ctx := auditCtx()
		f := setupTestFixture(t)

		session, refreshToken, err := auth.NewSession(ctx, uuid.New(), f.cfg.SessionTTL)
		require.NoError(t, err)
		session.ExpiresAt = time.Now().Add(-time.Hour) // Set to past

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.authRepo.EXPECT().
					GetSessionByRefreshTokenHash(ctx, mock.AnythingOfType("[]uint8")).
					Return(session, nil)
				return fn(ctx)
			})

		tokenPair, err := f.service.RefreshAccessToken(ctx, refreshToken)

		assert.Error(t, err)
		assert.ErrorIs(t, err, auth.ErrSessionInvalid)
		assert.Nil(t, tokenPair)
	})
}

func TestService_Logout(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := auditCtx()
		f := setupTestFixture(t)

		session, _, err := auth.NewSession(ctx, uuid.New(), f.cfg.SessionTTL)
		require.NoError(t, err)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.authRepo.EXPECT().GetSessionByID(ctx, session.ID).Return(session, nil)
				f.authRepo.EXPECT().UpdateSession(ctx, mock.AnythingOfType("*auth.Session")).Return(nil)
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeSession(mock.Anything, session.ID).Return(nil)

		err = f.service.Logout(ctx, session.ID)
		assert.NoError(t, err)
	})

	t.Run("Access token revoke failure is best-effort", func(t *testing.T) {
		ctx := auditCtx()
		f := setupTestFixture(t)

		session, _, err := auth.NewSession(ctx, uuid.New(), f.cfg.SessionTTL)
		require.NoError(t, err)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.authRepo.EXPECT().GetSessionByID(ctx, session.ID).Return(session, nil)
				f.authRepo.EXPECT().UpdateSession(ctx, mock.AnythingOfType("*auth.Session")).Return(nil)
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeSession(mock.Anything, session.ID).Return(assert.AnError)

		err = f.service.Logout(ctx, session.ID)
		assert.NoError(t, err)
	})

	t.Run("Session already expired", func(t *testing.T) {
		ctx := auditCtx()
		f := setupTestFixture(t)

		// Create valid session and manually set it as expired
		session, _, err := auth.NewSession(ctx, uuid.New(), f.cfg.SessionTTL)
		require.NoError(t, err)
		session.ExpiresAt = time.Now().Add(-time.Hour) // Set to past

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.authRepo.EXPECT().GetSessionByID(ctx, session.ID).Return(session, nil)
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeSession(mock.Anything, session.ID).Return(nil)

		err = f.service.Logout(ctx, session.ID)
		assert.NoError(t, err) // Should not error even if session is expired
	})
}

func TestService_RecoverPassword(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.RecoverPasswordInput{
			Email: "john@example.com",
		}

		mockUser := &user.User{
			ID:    uuid.New(),
			Name:  "John Doe",
			Email: input.Email,
		}

		f.throttler.EXPECT().
			TryAcquire(ctx, "recover-password:"+input.Email, auth.PasswordRecoveryThrottledTTL).
			Return(throttle.Result{Allowed: true}, nil)

		// Token persistence happens inside the transaction; the event is enqueued
		// AFTER the transaction commits (outside the closure).
		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			Run(func(ctx context.Context, fn func(context.Context) error) {
				f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(mockUser, nil)

				f.authRepo.EXPECT().
					StorePasswordRecoveryToken(ctx, mock.AnythingOfType("*auth.PasswordRecoveryToken")).
					Return(nil)
			}).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				return fn(ctx)
			})

		f.eventProducer.EXPECT().
			ProducePasswordRecoveryEvent(ctx, mock.AnythingOfType("auth.PasswordRecoveryMessage")).
			Return(nil)

		result, err := f.service.RecoverPassword(ctx, input)
		assert.NoError(t, err)
		require.True(t, result.Allowed)
	})

	t.Run("Event enqueue fails after commit", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.RecoverPasswordInput{
			Email: "john@example.com",
		}

		mockUser := &user.User{
			ID:    uuid.New(),
			Name:  "John Doe",
			Email: input.Email,
		}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			Run(func(ctx context.Context, fn func(context.Context) error) {
				f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(mockUser, nil)
				f.authRepo.EXPECT().
					StorePasswordRecoveryToken(ctx, mock.AnythingOfType("*auth.PasswordRecoveryToken")).
					Return(nil)
			}).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				return fn(ctx)
			})

		f.eventProducer.EXPECT().
			ProducePasswordRecoveryEvent(ctx, mock.AnythingOfType("auth.PasswordRecoveryMessage")).
			Return(assert.AnError)

		f.throttler.EXPECT().
			TryAcquire(ctx, "recover-password:"+input.Email, auth.PasswordRecoveryThrottledTTL).
			Return(throttle.Result{Allowed: true}, nil)

		_, err := f.service.RecoverPassword(ctx, input)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("Store token fails: event producer never called", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.RecoverPasswordInput{
			Email: "john@example.com",
		}

		mockUser := &user.User{
			ID:    uuid.New(),
			Name:  "John Doe",
			Email: input.Email,
		}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			Run(func(ctx context.Context, fn func(context.Context) error) {
				f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(mockUser, nil)
				f.authRepo.EXPECT().
					StorePasswordRecoveryToken(ctx, mock.AnythingOfType("*auth.PasswordRecoveryToken")).
					Return(assert.AnError)
			}).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				return fn(ctx)
			})

		// No event producer expectation: if RecoverPassword enqueued despite the tx failing,
		// the mock (constructed with t) would fail on an unexpected call.
		f.throttler.EXPECT().
			TryAcquire(ctx, "recover-password:"+input.Email, auth.PasswordRecoveryThrottledTTL).
			Return(throttle.Result{Allowed: true}, nil)

		_, err := f.service.RecoverPassword(ctx, input)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("Throttled", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.RecoverPasswordInput{
			Email: "john@example.com",
		}

		f.throttler.EXPECT().
			TryAcquire(ctx, "recover-password:"+input.Email, auth.PasswordRecoveryThrottledTTL).
			Return(throttle.Result{Allowed: false, RetryAfter: time.Now().Add(30 * time.Second)}, nil)

		result, err := f.service.RecoverPassword(ctx, input)
		assert.Error(t, err)
		assert.ErrorIs(t, err, auth.ErrPasswordRecoveryThrottled)
		assert.False(t, result.Allowed)
	})

	t.Run("Throttler fails", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.RecoverPasswordInput{
			Email: "john@example.com",
		}

		f.throttler.EXPECT().
			TryAcquire(ctx, "recover-password:"+input.Email, auth.PasswordRecoveryThrottledTTL).
			Return(throttle.Result{}, assert.AnError)

		_, err := f.service.RecoverPassword(ctx, input)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("User not found", func(t *testing.T) {
		ctx := context.Background()

		f := setupTestFixture(t)
		input := auth.RecoverPasswordInput{
			Email: "nonexistent@example.com",
		}

		f.throttler.EXPECT().
			TryAcquire(ctx, "recover-password:"+input.Email, auth.PasswordRecoveryThrottledTTL).
			Return(throttle.Result{Allowed: true}, nil)

		// GetByEmail error propagates out of the tx: unknown email -> ErrNotFound -> 404.
		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(nil, apperr.ErrNotFound)
				return fn(ctx)
			})

		_, err := f.service.RecoverPassword(ctx, input)
		assert.Error(t, err)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})
}

func TestService_ResetPassword(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()

		f := setupTestFixture(t)

		userID := uuid.New()
		tokenObj, tokenRaw, err := auth.NewPasswordRecoveryToken(userID, f.cfg.PasswordRecoveryTokenTTL)
		require.NoError(t, err)

		input := auth.ResetPasswordInput{
			Token:       tokenRaw,
			NewPassword: "newpassword123",
		}

		mockUser := &user.User{
			ID:    userID,
			Name:  "John Doe",
			Email: "john@example.com",
		}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			Run(func(ctx context.Context, fn func(context.Context) error) {
				f.authRepo.EXPECT().GetPasswordRecoveryToken(ctx, mock.AnythingOfType("[]uint8")).Return(tokenObj, nil)

				f.userRepo.EXPECT().GetByID(ctx, userID).Return(mockUser, nil)

				f.authRepo.EXPECT().
					UpdatePasswordRecoveryToken(ctx, mock.AnythingOfType("*auth.PasswordRecoveryToken")).
					Return(nil)

				f.userRepo.EXPECT().Update(ctx, mock.AnythingOfType("*user.User")).Return(nil)

				f.authRepo.EXPECT().RevokeUserSessions(ctx, userID).Return(nil)
			}).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeAllForUser(mock.Anything, userID).Return(nil)

		err = f.service.ResetPassword(ctx, input)
		assert.NoError(t, err)
	})

	t.Run("Access-token revoke failure still succeeds", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		userID := uuid.New()
		tokenObj, tokenRaw, err := auth.NewPasswordRecoveryToken(userID, f.cfg.PasswordRecoveryTokenTTL)
		require.NoError(t, err)

		input := auth.ResetPasswordInput{
			Token:       tokenRaw,
			NewPassword: "newpassword123",
		}

		mockUser := &user.User{ID: userID, Name: "John Doe", Email: "john@example.com"}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			Run(func(ctx context.Context, fn func(context.Context) error) {
				f.authRepo.EXPECT().GetPasswordRecoveryToken(ctx, mock.AnythingOfType("[]uint8")).Return(tokenObj, nil)
				f.userRepo.EXPECT().GetByID(ctx, userID).Return(mockUser, nil)
				f.authRepo.EXPECT().
					UpdatePasswordRecoveryToken(ctx, mock.AnythingOfType("*auth.PasswordRecoveryToken")).
					Return(nil)
				f.userRepo.EXPECT().Update(ctx, mock.AnythingOfType("*user.User")).Return(nil)
				f.authRepo.EXPECT().RevokeUserSessions(ctx, userID).Return(nil)
			}).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeAllForUser(mock.Anything, userID).Return(assert.AnError)

		err = f.service.ResetPassword(ctx, input)
		assert.NoError(t, err)
	})

	t.Run("Token expired", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		userID := uuid.New()
		tokenObj, tokenRaw, err := auth.NewPasswordRecoveryToken(userID, f.cfg.PasswordRecoveryTokenTTL)
		require.NoError(t, err)
		tokenObj.ExpiresAt = time.Now().Add(-time.Hour) // Set to past

		input := auth.ResetPasswordInput{
			Token:       tokenRaw,
			NewPassword: "newpassword123",
		}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			Run(func(ctx context.Context, fn func(context.Context) error) {
				f.authRepo.EXPECT().GetPasswordRecoveryToken(ctx, mock.AnythingOfType("[]uint8")).Return(tokenObj, nil)
			}).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				return fn(ctx)
			})

		err = f.service.ResetPassword(ctx, input)

		assert.Error(t, err)
		assert.ErrorIs(t, err, auth.ErrInvalidPasswordRecoveryToken)
	})
}

func TestService_ChangePassword(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		f := setupTestFixture(t)

		userID := uuid.New()
		ctx := rbac.WithContext(context.Background(), rbac.Context{
			Actor: rbac.Actor{UserID: &userID, Role: rbac.RoleUser},
		})
		oldPassword := "oldpassword123"
		newPassword := "newpassword123"

		hashedOldPassword, err := auth.HashPassword(oldPassword)
		require.NoError(t, err)

		mockUser := &user.User{
			ID:       userID,
			Name:     "John Doe",
			Email:    "john@example.com",
			Password: string(hashedOldPassword),
		}

		input := auth.ChangePasswordInput{
			Password:    oldPassword,
			NewPassword: newPassword,
		}

		f.userRepo.EXPECT().GetByID(ctx, userID).Return(mockUser, nil)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.userRepo.EXPECT().Update(ctx, mock.AnythingOfType("*user.User")).Return(nil)
				f.authRepo.EXPECT().RevokeUserSessions(ctx, userID).Return(nil)
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeAllForUser(mock.Anything, userID).Return(nil)

		err = f.service.ChangePassword(ctx, userID, input)
		assert.NoError(t, err)
	})

	t.Run("Access-token revoke failure still succeeds", func(t *testing.T) {
		f := setupTestFixture(t)

		userID := uuid.New()
		ctx := rbac.WithContext(context.Background(), rbac.Context{
			Actor: rbac.Actor{UserID: &userID, Role: rbac.RoleUser},
		})
		oldPassword := "oldpassword123"

		hashedOldPassword, err := auth.HashPassword(oldPassword)
		require.NoError(t, err)

		mockUser := &user.User{
			ID:       userID,
			Name:     "John Doe",
			Email:    "john@example.com",
			Password: string(hashedOldPassword),
		}

		input := auth.ChangePasswordInput{Password: oldPassword, NewPassword: "newpassword123"}

		f.userRepo.EXPECT().GetByID(ctx, userID).Return(mockUser, nil)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.userRepo.EXPECT().Update(ctx, mock.AnythingOfType("*user.User")).Return(nil)
				f.authRepo.EXPECT().RevokeUserSessions(ctx, userID).Return(nil)
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeAllForUser(mock.Anything, userID).Return(assert.AnError)

		err = f.service.ChangePassword(ctx, userID, input)
		assert.NoError(t, err)
	})

	t.Run("Wrong current password", func(t *testing.T) {
		f := setupTestFixture(t)

		userID := uuid.New()
		ctx := rbac.WithContext(context.Background(), rbac.Context{
			Actor: rbac.Actor{UserID: &userID, Role: rbac.RoleUser},
		})
		oldPassword := "oldpassword123"
		wrongPassword := "wrongpassword"

		hashedOldPassword, err := auth.HashPassword(oldPassword)
		require.NoError(t, err)

		mockUser := &user.User{
			ID:       userID,
			Name:     "John Doe",
			Email:    "john@example.com",
			Password: string(hashedOldPassword),
		}

		input := auth.ChangePasswordInput{
			Password:    wrongPassword,
			NewPassword: "newpassword123",
		}

		f.userRepo.EXPECT().GetByID(ctx, userID).Return(mockUser, nil)

		err = f.service.ChangePassword(ctx, userID, input)
		assert.Error(t, err)
		assert.ErrorIs(t, err, auth.ErrWrongCredentials)
	})
}

func TestService_RevokeUserSessions(t *testing.T) {
	adminCtx := func() context.Context {
		adminID := uuid.New()
		return rbac.WithContext(context.Background(), rbac.Context{
			Actor: rbac.Actor{UserID: &adminID, Role: rbac.RoleAdmin},
		})
	}

	t.Run("Admin revokes sessions in tx and tokens after commit", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)
		targetID := uuid.New()

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.authRepo.EXPECT().RevokeUserSessions(ctx, targetID).Return(nil)
				f.audit.EXPECT().Record(ctx, mock.MatchedBy(func(e audit.Entry) bool {
					return e.Action == auth.ActionRevokeUserSessions && e.EntityID == targetID.String()
				})).Return(nil)
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeAllForUser(mock.Anything, targetID).Return(nil)

		err := f.service.RevokeUserSessions(ctx, targetID)
		assert.NoError(t, err)
	})

	t.Run("Plain user is forbidden", func(t *testing.T) {
		userID := uuid.New()
		ctx := rbac.WithContext(context.Background(), rbac.Context{
			Actor: rbac.Actor{UserID: &userID, Role: rbac.RoleUser},
		})
		f := setupTestFixture(t)

		// No transactor/revoker expectations: authorization must reject before
		// any write path runs.
		err := f.service.RevokeUserSessions(ctx, uuid.New())
		assert.ErrorIs(t, err, rbac.ErrUnauthorizedPermission)
	})

	t.Run("Access-token revoke failure is returned", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)
		targetID := uuid.New()

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.authRepo.EXPECT().RevokeUserSessions(ctx, targetID).Return(nil)
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeAllForUser(mock.Anything, targetID).Return(assert.AnError)

		err := f.service.RevokeUserSessions(ctx, targetID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_RevokeSession(t *testing.T) {
	actorCtx := func(role rbac.Role, userID uuid.UUID) context.Context {
		return rbac.WithContext(context.Background(), rbac.Context{
			Actor: rbac.Actor{UserID: &userID, Role: role},
		})
	}

	t.Run("Self revokes own session", func(t *testing.T) {
		ownerID := uuid.New()
		ctx := actorCtx(rbac.RoleUser, ownerID)
		f := setupTestFixture(t)

		session, _, err := auth.NewSession(auditCtx(), ownerID, f.cfg.SessionTTL)
		require.NoError(t, err)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.authRepo.EXPECT().GetSessionByID(ctx, session.ID).Return(session, nil)
				f.authRepo.EXPECT().UpdateSession(ctx, mock.MatchedBy(func(s *auth.Session) bool {
					return s.ID == session.ID && s.RevokedAt.NotNull()
				})).Return(nil)
				f.audit.EXPECT().Record(ctx, mock.MatchedBy(func(e audit.Entry) bool {
					return e.Action == auth.ActionRevokeSession &&
						e.Entity == "session" &&
						e.EntityID == session.ID.String()
				})).Return(nil)
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeSession(mock.Anything, session.ID).Return(nil)

		err = f.service.RevokeSession(ctx, session.ID)
		assert.NoError(t, err)
	})

	t.Run("Plain user cannot revoke another user's session", func(t *testing.T) {
		actorID := uuid.New()
		ownerID := uuid.New()
		ctx := actorCtx(rbac.RoleUser, actorID)
		f := setupTestFixture(t)

		session, _, err := auth.NewSession(auditCtx(), ownerID, f.cfg.SessionTTL)
		require.NoError(t, err)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.authRepo.EXPECT().GetSessionByID(ctx, session.ID).Return(session, nil)
				return fn(ctx)
			})

		// No UpdateSession or revoker expectations (strict mocks fail on an
		// unexpected call); the audit recorder also must see zero calls.
		err = f.service.RevokeSession(ctx, session.ID)
		assert.ErrorIs(t, err, rbac.ErrUnauthorizedPermission)
		f.audit.AssertNumberOfCalls(t, "Record", 0)
	})

	t.Run("Admin revokes another user's session", func(t *testing.T) {
		adminID := uuid.New()
		ownerID := uuid.New()
		ctx := actorCtx(rbac.RoleAdmin, adminID)
		f := setupTestFixture(t)

		session, _, err := auth.NewSession(auditCtx(), ownerID, f.cfg.SessionTTL)
		require.NoError(t, err)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.authRepo.EXPECT().GetSessionByID(ctx, session.ID).Return(session, nil)
				f.authRepo.EXPECT().UpdateSession(ctx, mock.MatchedBy(func(s *auth.Session) bool {
					return s.ID == session.ID && s.RevokedAt.NotNull()
				})).Return(nil)
				f.audit.EXPECT().Record(ctx, mock.MatchedBy(func(e audit.Entry) bool {
					return e.Action == auth.ActionRevokeSession && e.EntityID == session.ID.String()
				})).Return(nil)
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeSession(mock.Anything, session.ID).Return(nil)

		err = f.service.RevokeSession(ctx, session.ID)
		assert.NoError(t, err)
	})

	t.Run("Not found propagates without writes", func(t *testing.T) {
		actorID := uuid.New()
		ctx := actorCtx(rbac.RoleUser, actorID)
		f := setupTestFixture(t)
		sessionID := uuid.New()

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.authRepo.EXPECT().GetSessionByID(ctx, sessionID).Return(nil, apperr.ErrNotFound)
				return fn(ctx)
			})

		err := f.service.RevokeSession(ctx, sessionID)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})

	t.Run("Post-commit revoker error is returned", func(t *testing.T) {
		ownerID := uuid.New()
		ctx := actorCtx(rbac.RoleUser, ownerID)
		f := setupTestFixture(t)

		session, _, err := auth.NewSession(auditCtx(), ownerID, f.cfg.SessionTTL)
		require.NoError(t, err)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.authRepo.EXPECT().GetSessionByID(ctx, session.ID).Return(session, nil)
				f.authRepo.EXPECT().UpdateSession(ctx, mock.AnythingOfType("*auth.Session")).Return(nil)
				return fn(ctx)
			})

		f.revoker.EXPECT().RevokeSession(mock.Anything, session.ID).Return(assert.AnError)

		err = f.service.RevokeSession(ctx, session.ID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_GetPasswordRecoveryToken(t *testing.T) {
	t.Run("Token Not Exists", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		rawToken := "nonexistent-token"

		f.authRepo.EXPECT().
			GetPasswordRecoveryToken(ctx, mock.AnythingOfType("[]uint8")).
			Return(nil, auth.ErrInvalidPasswordRecoveryToken)

		token, err := f.service.GetPasswordRecoveryToken(ctx, rawToken)

		assert.Error(t, err)
		assert.ErrorIs(t, err, auth.ErrInvalidPasswordRecoveryToken)
		assert.Nil(t, token)
	})
}

func TestService_RefreshAccessToken_SessionRevoked(t *testing.T) {
	ctx := auditCtx()
	f := setupTestFixture(t)

	session, refreshToken, err := auth.NewSession(ctx, uuid.New(), f.cfg.SessionTTL)
	require.NoError(t, err)
	session.RevokedAt.Set(time.Now(), false) // Revoke the session

	f.transactor.EXPECT().
		Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
		RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			f.authRepo.EXPECT().
				GetSessionByRefreshTokenHash(ctx, mock.AnythingOfType("[]uint8")).
				Return(session, nil)
			return fn(ctx)
		})

	tokenPair, err := f.service.RefreshAccessToken(ctx, refreshToken)

	assert.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrSessionInvalid)
	assert.Nil(t, tokenPair)
}

func TestService_ResetPassword_TokenNotFound(t *testing.T) {
	ctx := context.Background()
	f := setupTestFixture(t)

	input := auth.ResetPasswordInput{
		Token:       "nonexistent-token",
		NewPassword: "newpassword123",
	}

	f.transactor.EXPECT().
		Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
		RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			f.authRepo.EXPECT().
				GetPasswordRecoveryToken(ctx, mock.AnythingOfType("[]uint8")).
				Return(nil, apperr.ErrNotFound)
			return fn(ctx)
		})

	err := f.service.ResetPassword(ctx, input)

	assert.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidPasswordRecoveryToken)
}

func TestService_ResetPassword_TokenAlreadyUsed(t *testing.T) {
	ctx := context.Background()
	f := setupTestFixture(t)

	userID := uuid.New()
	tokenObj, tokenRaw, err := auth.NewPasswordRecoveryToken(userID, f.cfg.PasswordRecoveryTokenTTL)
	require.NoError(t, err)
	tokenObj.Use() // Mark as used

	input := auth.ResetPasswordInput{
		Token:       tokenRaw,
		NewPassword: "newpassword123",
	}

	f.transactor.EXPECT().
		Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
		RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			f.authRepo.EXPECT().
				GetPasswordRecoveryToken(ctx, mock.AnythingOfType("[]uint8")).
				Return(tokenObj, nil)
			return fn(ctx)
		})

	err = f.service.ResetPassword(ctx, input)

	assert.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidPasswordRecoveryToken)
}

func TestService_ChangePassword_UserNotFound(t *testing.T) {
	f := setupTestFixture(t)

	userID := uuid.New()
	ctx := rbac.WithContext(context.Background(), rbac.Context{
		Actor: rbac.Actor{UserID: &userID, Role: rbac.RoleUser},
	})
	input := auth.ChangePasswordInput{
		Password:    "oldpassword123",
		NewPassword: "newpassword123",
	}

	f.userRepo.EXPECT().GetByID(ctx, userID).Return(nil, apperr.ErrNotFound)

	err := f.service.ChangePassword(ctx, userID, input)

	assert.Error(t, err)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestGenerateOpaqueToken_InvalidSize(t *testing.T) {
	token, err := auth.GenerateOpaqueToken(0, "")
	assert.Error(t, err)
	assert.Empty(t, token)
	assert.Contains(t, err.Error(), "invalid token size")
}

func TestGenerateOpaqueToken_WithPrefix(t *testing.T) {
	token, err := auth.GenerateOpaqueToken(32, "rt")
	assert.NoError(t, err)
	assert.Contains(t, token, "rt_")
}

func TestVerifyAccessToken_InvalidSignature(t *testing.T) {
	userID := uuid.New()
	sessID := uuid.New()
	token, err := auth.SignAccessToken("correct-secret", time.Hour, userID, sessID, rbac.RoleAdmin)
	require.NoError(t, err)

	_, err = auth.VerifyAccessToken("wrong-secret", token)
	assert.Error(t, err)
}

type testFixture struct {
	transactor    *sharedMocks.Transactor
	userRepo      *mocks.UserRepository
	authRepo      *mocks.Repository
	eventProducer *mocks.EventProducer
	throttler     *sharedMocks.Throttler
	audit         *sharedMocks.Recorder
	revoker       *sharedMocks.Revoker
	service       *auth.Service
	cfg           config.Auth
}

func setupTestFixture(t *testing.T) *testFixture {
	authCfg := config.Auth{
		JwtSecret:                "test-secret",
		JwtTTL:                   time.Hour,
		SessionTTL:               24 * time.Hour,
		PasswordRecoveryTokenTTL: time.Hour,
		RegistrationTokenTTL:     time.Hour,
	}
	// InternalMode defaults to false, so registration is public in tests.
	cfg := &config.Config{Auth: authCfg}

	tr := sharedMocks.NewTransactor(t)
	userRepo := mocks.NewUserRepository(t)
	authRepo := mocks.NewRepository(t)
	eventProducer := mocks.NewEventProducer(t)
	throttler := sharedMocks.NewThrottler(t)
	auditRec := sharedMocks.NewRecorder(t)
	revoker := sharedMocks.NewRevoker(t)

	// Audit is a side-effect; most tests don't care. Best-effort paths call it
	// without a prior expectation, so register a lenient default. Tests that
	// assert audit behavior can still add a specific (more-recently-registered)
	// expectation, which testify matches first.
	auditRec.On("Record", mock.Anything, mock.AnythingOfType("audit.Entry")).Return(nil).Maybe()

	service := auth.NewService(cfg, tr, userRepo, authRepo, rbac.NewAuthorizer(), eventProducer, throttler, auditRec, revoker)

	t.Cleanup(func() {
		tr.AssertExpectations(t)
		userRepo.AssertExpectations(t)
		authRepo.AssertExpectations(t)
		eventProducer.AssertExpectations(t)
		throttler.AssertExpectations(t)
		auditRec.AssertExpectations(t)
		revoker.AssertExpectations(t)
	})

	return &testFixture{
		cfg:           authCfg,
		transactor:    tr,
		userRepo:      userRepo,
		authRepo:      authRepo,
		eventProducer: eventProducer,
		throttler:     throttler,
		audit:         auditRec,
		revoker:       revoker,
		service:       service,
	}
}
