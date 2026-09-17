// Package user provides the data model and business logic for managing users in system.
package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/prawirdani/golang-restapi/internal/ports/repository"
	"github.com/prawirdani/golang-restapi/internal/ports/storage"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

// Authorization vocabulary for the user entity. Permissions (coarse gates) and
// audit Actions (fine-grained events) share the "<entity>.<verb>[-<object>]"
// grammar. Each Action is gated by the permission carrying its coarse verb, so
// they are declared side by side to keep the two layers aligned.
//
// Only permissions with an enforcement point are declared: user creation is
// gated in the auth domain (auth.register-user) and there is no delete-user
// operation, so no user.create/user.delete here.
const (
	PermRead   rbac.Permission = "user.read"
	PermUpdate rbac.Permission = "user.update"

	// Update-class actions, all gated by PermUpdate.
	ActionUpdate               audit.Action = "user.update"
	ActionChangeProfilePicture audit.Action = "user.change-profile-picture"
	ActionDeleteProfilePicture audit.Action = "user.delete-profile-picture"
)

var permTables = rbac.PermissionTable{
	rbac.RoleSystem: {PermRead: {}, PermUpdate: {}},
	rbac.RoleAdmin:  {PermRead: {}, PermUpdate: {}},
	rbac.RoleUser:   {}, // Self read and update through authorize.SelfOr
}

type Service struct {
	transactor   repository.Transactor
	userRepo     Repository
	imageStorage storage.Storage
	authorizer   rbac.Authorizer
	audit        audit.Recorder
}

func NewService(
	transactor repository.Transactor,
	userRepo Repository,
	imageStorage storage.Storage,
	authorizer rbac.Authorizer,
	auditRecorder audit.Recorder,
) *Service {
	authorizer.RegisterPermissions(permTables)

	return &Service{
		transactor:   transactor,
		userRepo:     userRepo,
		imageStorage: imageStorage,
		authorizer:   authorizer,
		audit:        auditRecorder,
	}
}

func (s *Service) ListUser(
	ctx context.Context,
	filter *Filter,
) ([]User, repository.PaginationMeta, error) {
	if err := s.authorizer.Require(ctx, PermRead); err != nil {
		return nil, repository.PaginationMeta{}, err
	}
	return s.userRepo.List(ctx, filter)
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
