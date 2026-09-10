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
	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/auth/mocks"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	sharedMocks "github.com/prawirdani/golang-restapi/internal/testing/mocks"
	"github.com/prawirdani/golang-restapi/internal/throttle"
	"github.com/prawirdani/golang-restapi/internal/user"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

func init() {
	log.SetLogger(log.EmptyLog)
}

func TestService_Register(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := user.CreateUserInput{
			Name:     "John Doe",
			Email:    "john@example.com",
			Phone:    "1234567890",
			Password: "password123",
		}

		f.userRepo.EXPECT().Store(ctx, mock.AnythingOfType("*user.User")).Return(nil)

		err := f.service.Register(ctx, input)

		assert.NoError(t, err)
	})

	t.Run("Email already exists", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := user.CreateUserInput{
			Name:     "John Doe",
			Email:    "john@example.com",
			Password: "password123",
		}

		// Relies on the unique constraint: Store maps the violation to ErrEmailConflict.
		f.userRepo.EXPECT().Store(ctx, mock.AnythingOfType("*user.User")).Return(user.ErrEmailConflict)

		err := f.service.Register(ctx, input)

		assert.Error(t, err)
		assert.ErrorIs(t, err, user.ErrEmailConflict)
	})
}

func TestService_Login(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.LoginInput{
			Email:    "john@example.com",
			Password: "password123",
			Meta:     auth.SessionMeta{UserAgent: "test-agent"},
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
		f.authRepo.EXPECT().StoreSession(ctx, mock.AnythingOfType("*auth.Session")).Return(nil)
		f.authRepo.EXPECT().PruneExpiredUserSessions(ctx, mockUser.ID).Return(nil)

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
		ctx := context.Background()
		f := setupTestFixture(t)

		session, refreshToken, err := auth.NewSession(uuid.New(), "test-agent", net.ParseIP("203.0.113.5"), f.cfg.SessionTTL)
		require.NoError(t, err)

		prevRefreshToken := refreshToken
		meta := auth.SessionMeta{UserAgent: "new-agent", IPAddr: net.ParseIP("198.51.100.7")}

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

		tokenPair, err := f.service.RefreshAccessToken(ctx, prevRefreshToken, meta)

		assert.NoError(t, err)
		assert.NotNil(t, tokenPair)
		assert.NotEqual(t, prevRefreshToken, tokenPair.RefreshToken)
	})

	t.Run("Session expired", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		session, refreshToken, err := auth.NewSession(uuid.New(), "test-agent", net.ParseIP("203.0.113.5"), f.cfg.SessionTTL)
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

		tokenPair, err := f.service.RefreshAccessToken(ctx, refreshToken, auth.SessionMeta{})

		assert.Error(t, err)
		assert.ErrorIs(t, err, auth.ErrSessionInvalid)
		assert.Nil(t, tokenPair)
	})
}

func TestService_Logout(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		session, _, err := auth.NewSession(uuid.New(), "test-agent", net.ParseIP("203.0.113.5"), f.cfg.SessionTTL)
		require.NoError(t, err)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.authRepo.EXPECT().GetSessionByID(ctx, session.ID).Return(session, nil)
				f.authRepo.EXPECT().UpdateSession(ctx, mock.AnythingOfType("*auth.Session")).Return(nil)
				return fn(ctx)
			})

		err = f.service.Logout(ctx, session.ID)
		assert.NoError(t, err)
	})

	t.Run("Session already expired", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		// Create valid session and manually set it as expired
		session, _, err := auth.NewSession(uuid.New(), "test-agent", net.ParseIP("203.0.113.5"), f.cfg.SessionTTL)
		require.NoError(t, err)
		session.ExpiresAt = time.Now().Add(-time.Hour) // Set to past

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.authRepo.EXPECT().GetSessionByID(ctx, session.ID).Return(session, nil)
				return fn(ctx)
			})

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
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.userRepo.EXPECT().Update(ctx, mock.AnythingOfType("*user.User")).Return(nil)
				f.authRepo.EXPECT().RevokeUserSessions(ctx, userID).Return(nil)
				return fn(ctx)
			})

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
	ctx := context.Background()
	f := setupTestFixture(t)

	session, refreshToken, err := auth.NewSession(uuid.New(), "test-agent", net.ParseIP("203.0.113.5"), f.cfg.SessionTTL)
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

	tokenPair, err := f.service.RefreshAccessToken(ctx, refreshToken, auth.SessionMeta{})

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
	service       *auth.Service
	cfg           config.Auth
}

func setupTestFixture(t *testing.T) *testFixture {
	cfg := config.Auth{
		JwtSecret:                "test-secret",
		JwtTTL:                   time.Hour,
		SessionTTL:               24 * time.Hour,
		PasswordRecoveryTokenTTL: time.Hour,
	}
	tr := sharedMocks.NewTransactor(t)
	userRepo := mocks.NewUserRepository(t)
	authRepo := mocks.NewRepository(t)
	eventProducer := mocks.NewEventProducer(t)
	throttler := sharedMocks.NewThrottler(t)
	auditRec := sharedMocks.NewRecorder(t)

	// Audit is a side-effect; most tests don't care. Best-effort paths call it
	// without a prior expectation, so register a lenient default. Tests that
	// assert audit behavior can still add a specific (more-recently-registered)
	// expectation, which testify matches first.
	auditRec.On("Record", mock.Anything, mock.AnythingOfType("audit.Entry")).Return(nil).Maybe()

	service := auth.NewService(cfg, tr, userRepo, authRepo, rbac.NewAuthorizer(), eventProducer, throttler, auditRec)

	t.Cleanup(func() {
		tr.AssertExpectations(t)
		userRepo.AssertExpectations(t)
		authRepo.AssertExpectations(t)
		eventProducer.AssertExpectations(t)
		throttler.AssertExpectations(t)
		auditRec.AssertExpectations(t)
	})

	return &testFixture{
		cfg:           cfg,
		transactor:    tr,
		userRepo:      userRepo,
		authRepo:      authRepo,
		eventProducer: eventProducer,
		throttler:     throttler,
		audit:         auditRec,
		service:       service,
	}
}
