package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/domain"
	"github.com/prawirdani/golang-restapi/internal/domain/auth"
	"github.com/prawirdani/golang-restapi/internal/domain/auth/mocks"
	"github.com/prawirdani/golang-restapi/internal/domain/user"
	sharedMocks "github.com/prawirdani/golang-restapi/internal/testing/mocks"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

func init() {
	log.SetLogger(log.EmptyLog)
}

func TestService_Register(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.RegisterInput{
			Name:     "John Doe",
			Email:    "john@example.com",
			Phone:    "1234567890",
			Password: "password123",
		}

		f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(nil, domain.ErrNotFound)
		f.userRepo.EXPECT().Store(ctx, mock.AnythingOfType("*user.User")).Return(nil)

		err := f.service.Register(ctx, input)

		assert.NoError(t, err)
	})

	t.Run("Email already exists", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		input := auth.RegisterInput{
			Name:     "John Doe",
			Email:    "john@example.com",
			Password: "password123",
		}

		existingUser := &user.User{
			ID:    uuid.New(),
			Name:  "Existing User",
			Email: input.Email,
		}

		f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(existingUser, nil)

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
			Email:     "john@example.com",
			Password:  "password123",
			UserAgent: "test-agent",
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

		f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(nil, domain.ErrNotFound)

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

		session, refreshToken, err := auth.NewSession(uuid.New(), "test-agent", f.cfg.SessionTTL)
		require.NoError(t, err)

		prevRefreshToken := refreshToken

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.authRepo.EXPECT().
					GetSessionByRefreshTokenHash(ctx, mock.AnythingOfType("[]uint8")).
					Return(session, nil)
				f.authRepo.EXPECT().UpdateSession(ctx, session).Return(nil)

				return fn(ctx)
			})

		tokenPair, err := f.service.RefreshAccessToken(ctx, prevRefreshToken)

		assert.NoError(t, err)
		assert.NotNil(t, tokenPair)
		assert.NotEqual(t, prevRefreshToken, tokenPair.RefreshToken)
	})

	t.Run("Session expired", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		session, refreshToken, err := auth.NewSession(uuid.New(), "test-agent", f.cfg.SessionTTL)
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
		ctx := context.Background()
		f := setupTestFixture(t)

		session, _, err := auth.NewSession(uuid.New(), "test-agent", f.cfg.SessionTTL)
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
		session, _, err := auth.NewSession(uuid.New(), "test-agent", f.cfg.SessionTTL)
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

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			Run(func(ctx context.Context, fn func(context.Context) error) {
				f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(mockUser, nil)

				f.authRepo.EXPECT().
					StorePasswordRecoveryToken(ctx, mock.AnythingOfType("*auth.PasswordRecoveryToken")).
					Return(nil)

				f.publisher.EXPECT().
					SendPasswordRecoveryEmail(ctx, mock.AnythingOfType("auth.PasswordRecoveryEmailMessage")).
					Return(nil)
			}).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				return fn(ctx)
			})

		err := f.service.RecoverPassword(ctx, input)
		assert.NoError(t, err)
	})

	t.Run("User not found", func(t *testing.T) {
		ctx := context.Background()

		f := setupTestFixture(t)
		input := auth.RecoverPasswordInput{
			Email: "nonexistent@example.com",
		}

		// Mock expectations
		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			Run(func(ctx context.Context, fn func(context.Context) error) {
				f.userRepo.EXPECT().GetByEmail(ctx, input.Email).Return(nil, domain.ErrNotFound)
			}).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				return fn(ctx)
			})

		err := f.service.RecoverPassword(ctx, input)
		assert.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
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
		ctx := context.Background()
		f := setupTestFixture(t)

		userID := uuid.New()
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
		f.userRepo.EXPECT().Update(ctx, mock.AnythingOfType("*user.User")).Return(nil)

		err = f.service.ChangePassword(ctx, userID, input)
		assert.NoError(t, err)
	})

	t.Run("Wrong current password", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		userID := uuid.New()
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

type testFixture struct {
	transactor *sharedMocks.Transactor
	userRepo   *mocks.UserRepository
	authRepo   *mocks.Repository
	publisher  *mocks.MessagePublisher
	service    *auth.Service
	cfg        config.Auth
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
	msgPublisher := mocks.NewMessagePublisher(t)

	service := auth.NewService(cfg, tr, userRepo, authRepo, msgPublisher)

	t.Cleanup(func() {
		tr.AssertExpectations(t)
		userRepo.AssertExpectations(t)
		authRepo.AssertExpectations(t)
		msgPublisher.AssertExpectations(t)
	})

	return &testFixture{
		cfg:        cfg,
		transactor: tr,
		userRepo:   userRepo,
		authRepo:   authRepo,
		publisher:  msgPublisher,
		service:    service,
	}
}
