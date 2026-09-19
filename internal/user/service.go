// Package user provides the data model and business logic for managing users in system.
package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/prawirdani/golang-restapi/internal/ports/repository"
	"github.com/prawirdani/golang-restapi/internal/ports/revocation"
	"github.com/prawirdani/golang-restapi/internal/ports/storage"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

// Authorization vocabulary for the user entity. Permissions (coarse gates) and
// audit Actions (fine-grained events) share the "<entity>.<verb>[-<object>]"
// grammar. Each Action is gated by the permission carrying its coarse verb, so
// they are declared side by side to keep the two layers aligned.
//
// Only permissions with an enforcement point are declared. User creation is
// gated in the auth domain (auth.register-user), so there is no user.create
// here.
const (
	PermRead   rbac.Permission = "user.read"
	PermUpdate rbac.Permission = "user.update"
	PermDelete rbac.Permission = "user.delete"

	// Update-class actions, all gated by PermUpdate.
	ActionUpdate               audit.Action = "user.update"
	ActionDelete               audit.Action = "user.delete"
	ActionChangeProfilePicture audit.Action = "user.change-profile-picture"
	ActionDeleteProfilePicture audit.Action = "user.delete-profile-picture"
)

var permTables = rbac.PermissionTable{
	rbac.RoleSystem: {PermRead: {}, PermUpdate: {}, PermDelete: {}},
	rbac.RoleAdmin:  {PermRead: {}, PermUpdate: {}, PermDelete: {}},
	rbac.RoleUser:   {}, // Self read and update through authorize.SelfOr
}

// SessionRevoker revokes a user's persisted sessions. Implemented by the auth
// repository; declared here so the user service can revoke sessions inside its
// own transaction without depending on the auth package.
type SessionRevoker interface {
	// RevokeUserSessions revokes all active sessions belonging to userID.
	RevokeUserSessions(ctx context.Context, userID uuid.UUID) error
}

type Service struct {
	transactor     repository.Transactor
	userRepo       Repository
	imageStorage   storage.Storage
	authorizer     rbac.Authorizer
	audit          audit.Recorder
	sessionRevoker SessionRevoker
	revoker        revocation.Revoker
}

func NewService(
	transactor repository.Transactor,
	userRepo Repository,
	imageStorage storage.Storage,
	authorizer rbac.Authorizer,
	auditRecorder audit.Recorder,
	sessionRevoker SessionRevoker,
	revoker revocation.Revoker,
) *Service {
	authorizer.RegisterPermissions(permTables)

	return &Service{
		transactor:     transactor,
		userRepo:       userRepo,
		imageStorage:   imageStorage,
		authorizer:     authorizer,
		audit:          auditRecorder,
		sessionRevoker: sessionRevoker,
		revoker:        revoker,
	}
}

func (s *Service) ListUser(ctx context.Context, search *Search) ([]User, error) {
	if err := s.authorizer.Require(ctx, PermRead); err != nil {
		return nil, err
	}
	return s.userRepo.List(ctx, search)
}

func (s *Service) GetUserByID(ctx context.Context, userID uuid.UUID) (*User, error) {
	if err := s.authorizer.RequireSelfOr(ctx, userID, PermRead); err != nil {
		return nil, err
	}

	usr, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return usr, nil
}

func (s *Service) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	usr, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.RequireSelfOr(ctx, usr.ID, PermRead); err != nil {
		return nil, err
	}

	return usr, nil
}

// UpdateUser updates basic user's data (name and phone)
func (s *Service) UpdateUser(ctx context.Context, userID uuid.UUID, input UpdateUserInput) error {
	if err := s.authorizer.RequireSelfOr(ctx, userID, PermUpdate); err != nil {
		return err
	}
	return s.transactor.Transact(ctx, func(ctx context.Context) error {
		usr, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return err
		}
		prev := *usr // snapshot before mutation

		usr.Name = input.Name
		usr.Phone.Set(input.Phone, false)
		usr.Gender.Set(Gender(strings.ToUpper(input.Gender)), false)

		if err := usr.Validate(); err != nil {
			return err
		}

		if err := s.userRepo.Update(ctx, usr); err != nil {
			return err
		}

		return s.audit.Record(ctx, audit.Entry{
			Action:   ActionUpdate,
			Entity:   "user",
			EntityID: userID.String(),
			Prev:     prev,
			Next:     *usr,
		})
	})
}

func (s *Service) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	if err := s.authorizer.Require(ctx, PermDelete); err != nil {
		return err
	}

	// The not-found path is idempotent: it skips session rows (nothing to
	// revoke) but still writes the access-token watermark below.
	err := s.transactor.Transact(ctx, func(ctx context.Context) error {
		usr, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			// Idempotent
			if errors.Is(err, apperr.ErrNotFound) {
				return nil
			}
			return err
		}

		if err := s.userRepo.Delete(ctx, usr); err != nil {
			return err
		}

		// Repository joins this transaction via db.GetConn; persisting the
		// revocation makes a committed delete never leave sessions alive.
		if err := s.sessionRevoker.RevokeUserSessions(ctx, userID); err != nil {
			return err
		}

		return s.audit.Record(ctx, audit.Entry{
			Action:   ActionDelete,
			Entity:   "user",
			EntityID: usr.ID.String(),
			Prev:     nil,
			Next:     nil,
		})
	})
	if err != nil {
		return err
	}

	// Watermark stateless access tokens after the DB commit. This also runs on
	// the idempotent not-found path: a retry after a failed post-commit write
	// must converge and revoke the pre-delete access tokens. Unlike the session
	// rows this is the only record that access-token revocation happened, so its
	// failure is surfaced to the caller (the deletion itself stands).
	logger := log.GetFromContext(ctx).With("user_id", userID.String())
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := s.revoker.RevokeAllForUser(cleanupCtx, userID); err != nil {
		logger.Error("failed to revoke user access tokens", err)
		return err
	}

	return nil
}

func (s *Service) ChangeProfilePicture(
	ctx context.Context,
	userID uuid.UUID,
	file storage.File,
) error {
	if err := s.authorizer.RequireSelfOr(ctx, userID, PermUpdate); err != nil {
		return err
	}

	if err := file.SetName(uuid.NewString()); err != nil {
		return err
	}
	newImageName := file.Name()
	newImagePath := s.buildImagePath(newImageName)

	if err := s.imageStorage.Put(ctx, newImagePath, file, file.ContentType()); err != nil {
		return err
	}

	// -- Swap the image reference in the DB; capture previous path for cleanup.
	var prevImagePath string
	if err := s.transactor.Transact(ctx, func(ctx context.Context) error {
		u, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return err
		}

		if u.ProfilePicture.NotNull() {
			prevImagePath = s.buildImagePath(u.ProfilePicture.Get())
		}
		prev := *u

		u.ProfilePicture.Set(newImageName, false)
		if err := s.userRepo.Update(ctx, u); err != nil {
			return err
		}

		return s.audit.Record(ctx, audit.Entry{
			Action:   ActionChangeProfilePicture,
			Entity:   "user",
			EntityID: userID.String(),
			Prev:     prev,
			Next:     *u,
		})
	}); err != nil {
		prevImagePath = ""
		s.asyncDeleteImage(ctx, newImagePath, "rollback after failed db update")
		return err
	}

	if prevImagePath != "" {
		s.asyncDeleteImage(ctx, prevImagePath, "stale image cleanup")
	}
	return nil
}

func (s *Service) DeleteProfilePicture(ctx context.Context, userID uuid.UUID) error {
	if err := s.authorizer.RequireSelfOr(ctx, userID, PermUpdate); err != nil {
		return err
	}

	var prevImagePath string
	if err := s.transactor.Transact(ctx, func(ctx context.Context) error {
		u, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return err
		}

		if !u.ProfilePicture.NotNull() {
			return nil
		}
		prev := *u

		prevImagePath = s.buildImagePath(u.ProfilePicture.Get())
		u.ProfilePicture.Set("", false)

		if err := s.userRepo.Update(ctx, u); err != nil {
			return err
		}

		return s.audit.Record(ctx, audit.Entry{
			Action:   ActionDeleteProfilePicture,
			Entity:   "user",
			EntityID: userID.String(),
			Prev:     prev,
			Next:     *u,
		})
	}); err != nil {
		return err
	}

	if prevImagePath != "" {
		s.asyncDeleteImage(ctx, prevImagePath, "delete image")
	}

	return nil
}

const ImageStoragePath = "profiles"

func (s *Service) buildImagePath(imageName string) string {
	return fmt.Sprintf("%s/%s", ImageStoragePath, imageName)
}

// asyncDeleteImage spawns a short-lived goroutine to delete an object from storage.
// The logger is snapshotted from ctx so all request-scoped fields (trace-id, etc.)
// are preserved even after ctx is cancelled by the caller.
func (s *Service) asyncDeleteImage(ctx context.Context, path string, reason string) {
	logger := log.GetFromContext(ctx).With("image_path", path, "reason", reason)
	//nolint:gosec // G118: the cleanup goroutine intentionally outlives the request context
	go func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.imageStorage.Delete(cleanupCtx, path); err != nil {
			logger.Warn("failed to delete profile image", "error", err)
			return
		}
		logger.Debug("profile image deleted successfully")
	}()
}
