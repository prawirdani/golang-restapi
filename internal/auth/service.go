// Package auth provides authentication and authorization functionality.
// This package handles user authentication through sessions, access tokens, and
// password management including secure hashing and password reset flows. It manages
// the complete authentication lifecycle from login through logout, including token
// generation, validation, and session management.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/internal/repository"
	"github.com/prawirdani/golang-restapi/internal/throttle"
	"github.com/prawirdani/golang-restapi/internal/user"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

type Service struct {
	cfg           config.Auth
	transactor    repository.Transactor
	authRepo      Repository
	userRepo      UserRepository
	eventProducer EventProducer
	throttler     throttle.Throttler
}

func NewService(
	cfg config.Auth,
	transactor repository.Transactor,
	userRepo UserRepository,
	authRepo Repository,
	eventProducer EventProducer,
	throttler throttle.Throttler,
) *Service {
	return &Service{
		cfg:           cfg,
		transactor:    transactor,
		userRepo:      userRepo,
		authRepo:      authRepo,
		eventProducer: eventProducer,
		throttler:     throttler,
	}
}

func (s *Service) Register(ctx context.Context, inp user.CreateUserInput) error {
	hashedPassword, err := HashPassword(inp.Password)
	if err != nil {
		return err
	}

	newUser, err := user.New(
		inp.Name,
		inp.Email,
		inp.Phone,
		user.Gender(strings.ToUpper(inp.Gender)),
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
	usr, err := s.userRepo.GetByEmail(ctx, inp.Email)
	if err != nil {
		// Surface real DB errors (e.g. outage) instead of masking them as 401.
		if !errors.Is(err, apperr.ErrNotFound) {
			return nil, err
		}
		// Equalize timing with a dummy bcrypt compare so user enumeration
		// via response time is not possible.
		DummyVerify()
		return nil, ErrWrongCredentials
	}
	if usr == nil {
		DummyVerify()
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

	// ponytail: prune-on-login, no cron
	if err := s.authRepo.PruneExpiredUserSessions(ctx, usr.ID); err != nil {
		log.ErrorCtx(ctx, "Failed to prune expired sessions", err)
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
			if errors.Is(err, apperr.ErrNotFound) {
				return ErrSessionInvalid
			}
			return err
		}

		if sess.IsExpired() {
			return ErrSessionInvalid
		}

		if sess.RevokedAt.NotNull() {
			// ponytail: logs reuse signal; full token-family/history tracking out of scope (no schema change)
			log.WarnCtx(
				ctx, "Refresh attempt against revoked session; possible token reuse",
				"user_id", sess.UserID.String(),
				"session_id", sess.ID.String(),
			)
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

		if session.IsExpired() || session.RevokedAt.NotNull() {
			return nil
		}

		session.Revoke()

		return s.authRepo.UpdateSession(ctx, session)
	})
}

// RecoverPassword initiates the password recovery process by sending a reset link or token to the user's email.
func (s *Service) RecoverPassword(ctx context.Context, inp RecoverPasswordInput) (throttle.Result, error) {
	th, err := s.throttler.TryAcquire(ctx, fmt.Sprintf("recover-password:%s", inp.Email), PasswordRecoveryThrottledTTL)
	if err != nil {
		return th, err
	}

	if !th.Allowed {
		return th, ErrPasswordRecoveryThrottled.WithDetails(th)
	}

	var msg PasswordRecoveryMessage
	err = s.transactor.Transact(ctx, func(ctx context.Context) error {
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

		msg = PasswordRecoveryMessage{
			To:       usr.Email,
			Name:     usr.Name,
			ResetURL: s.cfg.ResetPasswordFormEndpoint + "?token=" + tokenRaw,
			Expiry:   s.cfg.PasswordRecoveryTokenTTL,
		}
		return nil
	})
	if err != nil {
		return th, err
	}

	if err := s.eventProducer.ProducePasswordRecoveryEvent(ctx, msg); err != nil {
		log.ErrorCtx(ctx, "Failed to enqueue password recovery email", err)
		return th, err
	}
	return th, nil
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
	sum := HashStr(inp.Token)
	return s.transactor.Transact(ctx, func(ctx context.Context) error {
		token, err := s.authRepo.GetPasswordRecoveryToken(ctx, sum)
		if err != nil {
			if errors.Is(err, apperr.ErrNotFound) {
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

		if err := s.userRepo.Update(ctx, user); err != nil {
			return err
		}

		return s.authRepo.RevokeUserSessions(ctx, user.ID)
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

	return s.transactor.Transact(ctx, func(ctx context.Context) error {
		if err := s.userRepo.Update(ctx, usr); err != nil {
			return err
		}
		// ponytail: revokes ALL sessions incl. current; pass current sessID to exempt if desired
		return s.authRepo.RevokeUserSessions(ctx, userID)
	})
}

func (s *Service) generateAccessToken(userID, sessID uuid.UUID) (string, error) {
	return SignAccessToken(s.cfg.JwtSecret, s.cfg.JwtTTL, userID, sessID)
}
