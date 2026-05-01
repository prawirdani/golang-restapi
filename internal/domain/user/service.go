// Package user provides the domain model and business logic for managing users in system.
package user

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/repository"
	"github.com/prawirdani/golang-restapi/internal/infrastructure/storage"
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

func (s *Service) ChangeProfilePicture(
	ctx context.Context,
	userID uuid.UUID,
	file storage.File,
) error {
	//  Prev image name + storage path for cleanup
	var prevImagePath string
	err := s.transactor.Transact(ctx, func(ctx context.Context) error {
		usr, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return err
		}

		if usr.ProfileImage.Valid() {
			prevImagePath = s.buildImagePath(usr.ProfileImage.Get())
		}

		//  Set New Image name using UUID
		if err := file.SetName(uuid.NewString()); err != nil {
			return err
		}

		newImageName := file.Name()
		newImagePath := s.buildImagePath(newImageName)

		//  Update user image_profile field and save to db
		usr.ProfileImage.Set(newImageName, false)
		if err := s.userRepo.Update(ctx, usr); err != nil {
			return err
		}

		//  Store new image to storage
		if err := s.imageStorage.Put(ctx, newImagePath, file, file.ContentType()); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	// -- Cleanup old image in async
	if prevImagePath != "" {
		logger := log.GetFromContext(ctx).With(
			"user_id", userID,
			"prev_image_path", prevImagePath,
		) // snapshot logger, it may contains useful fields
		go func(path string, logger log.Logger) {
			if err := s.imageStorage.Delete(context.Background(), path); err != nil {
				logger.Warn("Failed to cleanup previous profile image", "error", err.Error())
			} else {
				logger.Debug("Success clean up previous profile image")
			}
		}(prevImagePath, logger)
	}

	return nil
}

const ImageStoragePath = "profiles"

func (s *Service) buildImagePath(imageName string) string {
	return fmt.Sprintf("%s/%s", ImageStoragePath, imageName)
}
