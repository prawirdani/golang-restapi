// Package auth provides authentication and authorization functionality.
// This package handles user authentication through sessions, access tokens, and
// password management including secure hashing and password reset flows. It manages
// the complete authentication lifecycle from login through logout, including token
// generation, validation, and session management.
package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/domain"
	"github.com/prawirdani/golang-restapi/internal/domain/user"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/repository"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

type Service struct {
	cfg        config.Auth
	transactor repository.Transactor
	authRepo   Repository
	userRepo   UserRepository
	publisher  MessagePublisher
}

func NewService(
	cfg config.Auth,
	transactor repository.Transactor,
	userRepo UserRepository,
	authRepo Repository,
	publisher MessagePublisher,
) *Service {
	return &Service{
		cfg:        cfg,
		transactor: transactor,
		userRepo:   userRepo,
		authRepo:   authRepo,
		publisher:  publisher,
	}
}

func (s *Service) Register(ctx context.Context, inp RegisterInput) error {
	if userExists, _ := s.userRepo.GetByEmail(ctx, inp.Email); userExists != nil {
		return user.ErrEmailConflict
	}

	hashedPassword, err := HashPassword(inp.Password)
	if err != nil {
		return err
	}

	newUser, err := user.New(
		inp.Name,
		inp.Email,
		inp.Phone,
		string(hashedPassword),
	)
	if err != nil {
		return err
	}

	return s.userRepo.Store(ctx, newUser)
}

// Login is a method to authenticate the user, returning access token, refresh token, and error if any.
func (s *Service) Login(
	ctx context.Context,
	inp LoginInput,
) (*TokenPair, error) {
	usr, _ := s.userRepo.GetByEmail(ctx, inp.Email)
	if usr == nil {
		return nil, ErrWrongCredentials
	}

	if err := VerifyPassword(inp.Password, usr.Password); err != nil {
		return nil, err
	}

	sess, refreshToken, err := NewSession(
		usr.ID,
		inp.UserAgent,
		s.cfg.SessionTTL,
	)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.generateAccessToken(usr.ID, sess.ID)
	if err != nil {
		return nil, err
	}

	if err = s.authRepo.StoreSession(ctx, sess); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// RefreshAccessToken refresh access token. it will rotate the refresh token if refreshing success.
func (s *Service) RefreshAccessToken(
	ctx context.Context,
	refreshToken string,
) (*TokenPair, error) {
	sum := HashStr(refreshToken)

	tokenPair := new(TokenPair)
	err := s.transactor.Transact(ctx, func(ctx context.Context) error {
		sess, err := s.authRepo.GetSessionByRefreshTokenHash(ctx, sum)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return ErrSessionInvalid
			}
			return err
		}

		if sess.IsExpired() || sess.RevokedAt.Valid() {
			return ErrSessionInvalid
		}

		newAccessToken, err := s.generateAccessToken(sess.UserID, sess.ID)
		if err != nil {
			return err
		}
		tokenPair.AccessToken = newAccessToken

		// Rotate refreshToken
		newRefreshToken, err := sess.Rotate()
		if err != nil {
			return err
		}
		tokenPair.RefreshToken = newRefreshToken

		// Save session changes
		return s.authRepo.UpdateSession(ctx, sess)
	})
	if err != nil {
		return nil, err
	}

	return tokenPair, nil
}

func (s *Service) Logout(ctx context.Context, sessID uuid.UUID) error {
	return s.transactor.Transact(ctx, func(ctx context.Context) error {
		session, err := s.authRepo.GetSessionByID(ctx, sessID)
		if err != nil {
			return err
		}

		if session.IsExpired() || session.RevokedAt.Valid() {
			return nil
		}

		session.Revoke()

		return s.authRepo.UpdateSession(ctx, session)
	})
}

// RecoverPassword initiates the password recovery process by sending a reset link or token to the user's email.
func (s *Service) RecoverPassword(ctx context.Context, inp RecoverPasswordInput) error {
	return s.transactor.Transact(ctx, func(ctx context.Context) error {
		usr, err := s.userRepo.GetByEmail(ctx, inp.Email)
		if err != nil {
			return err
		}

		tokenObj, tokenRaw, err := NewPasswordRecoveryToken(usr.ID, s.cfg.PasswordRecoveryTokenTTL)
		if err != nil {
			log.ErrorCtx(ctx, "Failed to create reset password token", err)
			return err
		}

		// Save token to db
		if err := s.authRepo.StorePasswordRecoveryToken(ctx, tokenObj); err != nil {
			return err
		}

		// Publish email job to message queue
		msg := PasswordRecoveryEmailMessage{
			To:       usr.Email,
			Name:     usr.Name,
			ResetURL: s.cfg.ResetPasswordFormEndpoint + "?token=" + tokenRaw,
			Expiry:   s.cfg.PasswordRecoveryTokenTTL,
		}

		return s.publisher.SendPasswordRecoveryEmail(ctx, msg)
	})
}

func (s *Service) GetPasswordRecoveryToken(
	ctx context.Context,
	token string,
) (*PasswordRecoveryToken, error) {
	sum := HashStr(token)
	return s.authRepo.GetPasswordRecoveryToken(ctx, sum)
}

// ResetPassword resets a user's password using a valid password recovery token from email.
func (s *Service) ResetPassword(ctx context.Context, inp ResetPasswordInput) error {
	return s.transactor.Transact(ctx, func(ctx context.Context) error {
		sum := HashStr(inp.Token)
		token, err := s.authRepo.GetPasswordRecoveryToken(ctx, sum)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return ErrInvalidPasswordRecoveryToken
			}
			return err
		}

		if token.Expired() || token.IsUsed() {
			return ErrInvalidPasswordRecoveryToken
		}

		user, err := s.userRepo.GetByID(ctx, token.UserID)
		if err != nil {
			return err
		}

		newHashedPassword, err := HashPassword(inp.NewPassword)
		if err != nil {
			log.ErrorCtx(ctx, "Failed to hash new password", err)
			return err
		}
		user.Password = string(newHashedPassword)

		token.Use()
		if err := s.authRepo.UpdatePasswordRecoveryToken(ctx, token); err != nil {
			return err
		}

		return s.userRepo.Update(ctx, user)
	})
}

// ChangePassword updates the authenticated user's password after verifying the current password.
func (s *Service) ChangePassword(
	ctx context.Context,
	userID uuid.UUID,
	inp ChangePasswordInput,
) error {
	usr, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify old password
	if err := VerifyPassword(inp.Password, usr.Password); err != nil {
		return err
	}

	// Hash new password
	newHashedPassword, err := HashPassword(inp.NewPassword)
	if err != nil {
		return err
	}

	usr.Password = string(newHashedPassword)

	return s.userRepo.Update(ctx, usr)
}

func (s *Service) generateAccessToken(userID, sessID uuid.UUID) (string, error) {
	return SignAccessToken(s.cfg.JwtSecret, userID, sessID, s.cfg.JwtTTL)
}
