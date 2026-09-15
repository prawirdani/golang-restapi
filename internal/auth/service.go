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
	"strconv"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/prawirdani/golang-restapi/internal/ports/repository"
	"github.com/prawirdani/golang-restapi/internal/ports/throttle"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/internal/user"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

const (
	PermChangePassword rbac.Permission = "auth.change-password"
	PermRegisterUser   rbac.Permission = "auth.register-user" // new user registration by authorized roles (if internal mode is true)
)

// Audit Actions for the auth entity. Login/logout/change-password are gated by
// the coarse auth permissions; failure and reuse events have no gate (they
// record refused attempts) and are always best-effort.
//
//nolint:gosec // G101: audit action identifiers, not credentials
const (
	ActionRegister             audit.Action = "auth.register"
	ActionCompleteRegistration audit.Action = "auth.complete-registration"
	ActionLogin                audit.Action = "auth.login"
	ActionLoginFailed          audit.Action = "auth.login-failed"
	ActionLogout               audit.Action = "auth.logout"
	ActionChangePassword       audit.Action = "auth.change-password"
	ActionResetPassword        audit.Action = "auth.reset-password"
	ActionRecoverPassword      audit.Action = "auth.password-recovery-request"
	ActionTokenReuse           audit.Action = "auth.token-reuse-detected"
)

type Service struct {
	cfg           *config.Config
	transactor    repository.Transactor
	authRepo      Repository
	userRepo      UserRepository
	authorizer    rbac.Authorizer
	eventProducer EventProducer
	throttler     throttle.Throttler
	audit         audit.Recorder
}

func NewService(
	cfg *config.Config,
	transactor repository.Transactor,
	userRepo UserRepository,
	authRepo Repository,
	authorizer rbac.Authorizer,
	eventProducer EventProducer,
	throttler throttle.Throttler,
	auditRecorder audit.Recorder,
) *Service {
	permTables := rbac.PermissionTable{
		rbac.RoleSystem: {PermChangePassword: {}},
		rbac.RoleAdmin:  {PermChangePassword: {}},
		rbac.RoleUser:   {},
	}

	if cfg.App.InternalMode {
		permTables[rbac.RoleSystem][PermRegisterUser] = struct{}{}
		permTables[rbac.RoleAdmin][PermRegisterUser] = struct{}{}
	}

	authorizer.RegisterPermissions(permTables)

	return &Service{
		cfg:           cfg,
		transactor:    transactor,
		userRepo:      userRepo,
		authRepo:      authRepo,
		eventProducer: eventProducer,
		throttler:     throttler,
		authorizer:    authorizer,
		audit:         auditRecorder,
	}
}

// auditBestEffort records an audit entry without propagating failure to the
// caller. Used for events that must not block the response — failed attempts
// and unauthenticated flows where the action already refused or has no tx.
func (s *Service) auditBestEffort(ctx context.Context, e audit.Entry) {
	if err := s.audit.Record(ctx, e); err != nil {
		log.WarnCtx(ctx, "failed to record audit event", "action", string(e.Action), "error", err)
	}
}

// Register issues a short-lived registration token for inp.Email and
// sends a completion link once the token is durably stored. In internal
// mode it requires the PermRegisterUser permission (admin/system only).
// Returns user.ErrEmailConflict if the email is already registered.
func (s *Service) Register(ctx context.Context, inp RegisterInput) error {
	if s.cfg.App.InternalMode {
		if err := s.authorizer.Require(ctx, PermRegisterUser); err != nil {
			return err
		}
	}

	var token *RegistrationToken
	var rawToken string

	err := s.transactor.Transact(ctx, func(ctx context.Context) error {
		u, err := s.userRepo.GetByEmail(ctx, inp.Email)
		if err != nil && !errors.Is(err, apperr.ErrNotFound) {
			return fmt.Errorf("looking up user by email: %w", err)
		}
		if u != nil {
			return user.ErrEmailConflict
		}

		token, rawToken, err = NewRegistrationToken(inp.Name, inp.Email, s.cfg.Auth.RegistrationTokenTTL)
		if err != nil {
			return fmt.Errorf("creating registration token: %w", err)
		}

		if err := s.authRepo.StoreRegistrationToken(ctx, token); err != nil {
			return fmt.Errorf("storing registration token: %w", err)
		}

		return s.audit.Record(ctx, audit.Entry{
			Action:   ActionRegister,
			Entity:   "registration_token",
			EntityID: strconv.Itoa(token.ID),
			Prev:     nil,
			Next:     token,
			Meta:     nil,
		})
	})
	if err != nil {
		return err
	}

	if err := s.eventProducer.ProduceRegistrationCompletionEvent(ctx, CompleteRegistrationMessage{
		To:     inp.Email,
		Name:   inp.Name,
		URL:    fmt.Sprintf("%s?token=%s", s.cfg.Auth.CompleteRegistrationFormEndpoint, rawToken),
		Expiry: s.cfg.Auth.RegistrationTokenTTL,
	}); err != nil {
		return fmt.Errorf("registration succeeded but failed to send completion email: %w", err)
	}

	return nil
}

// CompleteRegistration consumes a valid registration token and creates the
// user with the chosen password, atomically marking the token used. Returns
// ErrInvalidRegistrationToken if the token is missing, expired, or already used.
func (s *Service) CompleteRegistration(ctx context.Context, inp CompleteRegistrationInput) error {
	passwordHash, err := HashPassword(inp.Password)
	if err != nil {
		return err
	}

	sum := HashStr(inp.Token)

	return s.transactor.Transact(ctx, func(ctx context.Context) error {
		regToken, err := s.authRepo.GetRegistrationToken(ctx, sum)
		if err != nil {
			//  an unknown token is an auth failure, not a 404.
			if errors.Is(err, apperr.ErrNotFound) {
				return ErrInvalidRegistrationToken
			}
			return err
		}

		// TODO: if used, return early to make it idempotent???
		if err := regToken.Use(); err != nil {
			return err
		}

		u, err := user.New(regToken.Name, regToken.Email, string(passwordHash))
		if err != nil {
			return err
		}

		// TODO: Prune all tokens that uses same email???
		if err := s.authRepo.UpdateRegistrationToken(ctx, regToken); err != nil {
			return err
		}

		if err := s.userRepo.Store(ctx, u); err != nil {
			return err
		}

		return s.audit.Record(ctx, audit.Entry{
			Action:   ActionCompleteRegistration,
			Entity:   "user",
			EntityID: u.ID.String(),
			Next:     u,
		})
	})
}

// GetRegistrationToken looks up a registration token by its raw value so the
// completion form can show the invitee's status (expiry / already used).
func (s *Service) GetRegistrationToken(ctx context.Context, rawToken string) (*RegistrationToken, error) {
	sum := HashStr(rawToken)
	return s.authRepo.GetRegistrationToken(ctx, sum)
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
		s.auditBestEffort(ctx, audit.Entry{
			Action:   ActionLoginFailed,
			Entity:   "user",
			EntityID: inp.Email,
			Meta:     map[string]any{"email": inp.Email},
		})
		return nil, ErrWrongCredentials
	}
	if usr == nil {
		DummyVerify()
		s.auditBestEffort(ctx, audit.Entry{
			Action:   ActionLoginFailed,
			Entity:   "user",
			EntityID: inp.Email,
			Meta:     map[string]any{"email": inp.Email},
		})
		return nil, ErrWrongCredentials
	}

	if err := VerifyPassword(inp.Password, usr.Password); err != nil {
		s.auditBestEffort(ctx, audit.Entry{
			Action:   ActionLoginFailed,
			Entity:   "user",
			EntityID: usr.ID.String(),
			Meta:     map[string]any{"email": inp.Email},
		})
		return nil, err
	}

	sess, refreshToken, err := NewSession(ctx, usr.ID, s.cfg.Auth.SessionTTL)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.generateAccessToken(usr.ID, sess.ID, usr.Role)
	if err != nil {
		return nil, err
	}

	if err := s.authRepo.StoreSession(ctx, sess); err != nil {
		return nil, err
	}

	s.auditBestEffort(ctx, audit.Entry{
		Action:   ActionLogin,
		Entity:   "user",
		EntityID: usr.ID.String(),
		Meta:     map[string]any{"session_id": sess.ID.String()},
	})

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
	var reusedUserID, reusedSessionID uuid.UUID
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
			reusedUserID = sess.UserID
			reusedSessionID = sess.ID
			return ErrSessionInvalid
		}

		u, err := s.userRepo.GetByID(ctx, sess.UserID)
		if err != nil {
			return err
		}

		newAccessToken, err := s.generateAccessToken(sess.UserID, sess.ID, u.Role)
		if err != nil {
			return err
		}
		tokenPair.AccessToken = newAccessToken

		// Rotate refreshToken
		newRefreshToken, err := sess.Rotate(ctx)
		if err != nil {
			return err
		}
		tokenPair.RefreshToken = newRefreshToken

		// Save session changes
		return s.authRepo.UpdateSession(ctx, sess)
	})
	if err != nil {
		// Best-effort audit outside the tx (it rolled back); the reuse signal
		// is the highest-value security event so it must not be lost.
		if reusedSessionID != uuid.Nil {
			s.auditBestEffort(ctx, audit.Entry{
				Action:   ActionTokenReuse,
				Entity:   "session",
				EntityID: reusedSessionID.String(),
				Meta:     map[string]any{"user_id": reusedUserID.String()},
			})
		}
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

		if err := s.authRepo.UpdateSession(ctx, session); err != nil {
			return err
		}

		return s.audit.Record(ctx, audit.Entry{
			Action:   ActionLogout,
			Entity:   "user",
			EntityID: session.UserID.String(),
			Meta:     map[string]any{"session_id": sessID.String()},
		})
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

		tokenObj, tokenRaw, err := NewPasswordRecoveryToken(usr.ID, s.cfg.Auth.PasswordRecoveryTokenTTL)
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
			ResetURL: s.cfg.Auth.ResetPasswordFormEndpoint + "?token=" + tokenRaw,
			Expiry:   s.cfg.Auth.PasswordRecoveryTokenTTL,
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

	s.auditBestEffort(ctx, audit.Entry{
		Action:   ActionRecoverPassword,
		Entity:   "user",
		EntityID: msg.To,
	})
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

		if err := s.authRepo.RevokeUserSessions(ctx, user.ID); err != nil {
			return err
		}

		return s.audit.Record(ctx, audit.Entry{
			Action:   ActionResetPassword,
			Entity:   "user",
			EntityID: user.ID.String(),
		})
	})
}

// ChangePassword updates the authenticated user's password after verifying the current password.
func (s *Service) ChangePassword(
	ctx context.Context,
	userID uuid.UUID,
	inp ChangePasswordInput,
) error {
	if err := s.authorizer.RequireSelfOr(ctx, userID, PermChangePassword); err != nil {
		return err
	}

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
		if err := s.authRepo.RevokeUserSessions(ctx, userID); err != nil {
			return err
		}

		return s.audit.Record(ctx, audit.Entry{
			Action:   ActionChangePassword,
			Entity:   "user",
			EntityID: userID.String(),
		})
	})
}

func (s *Service) ListPermission(ctx context.Context) ([]rbac.Permission, error) {
	return s.authorizer.ListPermission(ctx)
}

func (s *Service) generateAccessToken(userID, sessID uuid.UUID, role rbac.Role) (string, error) {
	return SignAccessToken(s.cfg.Auth.JwtSecret, s.cfg.Auth.JwtTTL, userID, sessID, role)
}
