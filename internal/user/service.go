// Package user provides the data model and business logic for managing users in system.
package user

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/repository"
	"github.com/prawirdani/golang-restapi/internal/storage"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

type Service struct {
	transactor   repository.Transactor
	userRepo     Repository
	imageStorage storage.Storage
}

func NewService(
	transactor repository.Transactor,
	userRepo Repository,
	imageStorage storage.Storage,
) *Service {
	return &Service{
		transactor:   transactor,
		userRepo:     userRepo,
		imageStorage: imageStorage,
	}
}

func (s *Service) GetUserByID(ctx context.Context, userID uuid.UUID) (*User, error) {
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

	return usr, nil
}

// UpdateUser updates basic user's data (name and phone)
func (s *Service) UpdateUser(ctx context.Context, userID uuid.UUID, input UpdateUserInput) error {
	return s.transactor.Transact(ctx, func(ctx context.Context) error {
		usr, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return err
		}

		usr.Name = input.Name
		usr.Phone.Set(input.Phone, false)
		usr.Gender.Set(Gender(strings.ToUpper(input.Gender)), false)

		if err := usr.Validate(); err != nil {
			return err
		}

		return s.userRepo.Update(ctx, usr)
	})
}

func (s *Service) ChangeProfilePicture(
	ctx context.Context,
	userID uuid.UUID,
	file storage.File,
) error {
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

		u.ProfilePicture.Set(newImageName, false)
		if err := s.userRepo.Update(ctx, u); err != nil {
			return err
		}
		return nil
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
	var prevImagePath string
	if err := s.transactor.Transact(ctx, func(ctx context.Context) error {
		u, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return err
		}

		if !u.ProfilePicture.NotNull() {
			return nil
		}

		prevImagePath = s.buildImagePath(u.ProfilePicture.Get())
		u.ProfilePicture.Set("", false)

		if err := s.userRepo.Update(ctx, u); err != nil {
			return err
		}

		return nil
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
