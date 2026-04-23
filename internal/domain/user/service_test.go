package user_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/domain"
	"github.com/prawirdani/golang-restapi/internal/domain/user"
	"github.com/prawirdani/golang-restapi/internal/domain/user/mocks"
	sharedMocks "github.com/prawirdani/golang-restapi/internal/testing/mocks"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/prawirdani/golang-restapi/pkg/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLogger(log.EmptyLog)
}

// Mirror the internal buildImagePath func
func buildImagePath(filename string) string {
	return fmt.Sprintf("%s/%s", user.ImageStoragePath, filename)
}

func TestNewUserService(t *testing.T) {
	f := setupTestFixture(t)
	require.NotNil(t, f.service)
}

func TestService_GetUserByID(t *testing.T) {
	t.Run("Without profile image", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		userID := uuid.New()
		expectedUser := &user.User{
			ID:           userID,
			Name:         "John Doe",
			Email:        "john@example.com",
			Password:     "hashedpassword",
			Phone:        nullable.New("123456789", false),
			ProfileImage: nullable.New("", false),
		}

		f.repo.EXPECT().GetByID(ctx, userID).Return(expectedUser, nil)

		u, err := f.service.GetUserByID(ctx, userID)
		assert.NoError(t, err)
		assert.Equal(t, expectedUser, u)
		assert.False(t, u.ProfileImage.Valid())
	})

	t.Run("With profile image", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		userID := uuid.New()
		expectedUser := &user.User{
			ID:           userID,
			Name:         "John Doe",
			Email:        "john@example.com",
			Password:     "hashedpassword",
			Phone:        nullable.New("123456789", false),
			ProfileImage: nullable.New("profile.jpg", false),
		}

		f.repo.EXPECT().GetByID(ctx, userID).Return(expectedUser, nil)

		result, err := f.service.GetUserByID(ctx, userID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestService_GetUserByEmail(t *testing.T) {
	t.Run("Without profile image", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		email := "john@example.com"
		expectedUser := &user.User{
			ID:           uuid.New(),
			Name:         "John Doe",
			Email:        email,
			Password:     "hashedpassword",
			Phone:        nullable.New("123456789", false),
			ProfileImage: nullable.New("", false),
		}

		f.repo.EXPECT().GetByEmail(ctx, email).Return(expectedUser, nil)

		result, err := f.service.GetUserByEmail(ctx, email)
		assert.NoError(t, err)
		assert.Equal(t, expectedUser, result)
		assert.False(t, result.ProfileImage.Valid())
	})

	t.Run("With profile image", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		email := "john@example.com"
		expectedUser := &user.User{
			ID:           uuid.New(),
			Name:         "John Doe",
			Email:        email,
			Password:     "hashedpassword",
			Phone:        nullable.New("123456789", false),
			ProfileImage: nullable.New("profile.jpg", false),
		}

		f.repo.EXPECT().GetByEmail(ctx, email).Return(expectedUser, nil)

		result, err := f.service.GetUserByEmail(ctx, email)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestService_ChangeProfilePicture(t *testing.T) {
	t.Run("Success user without existing profile image", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		existingUser := &user.User{
			ID:           uuid.New(),
			Name:         "John Doe",
			Email:        "john@example.com",
			Password:     "hashedpassword",
			Phone:        nullable.New("123456789", false),
			ProfileImage: nullable.New("", false),
		}

		/* ========================= internal op mock sequence ========================= */
		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, existingUser.ID).Return(existingUser, nil)

				var generatedFilename string
				f.file.EXPECT().SetName(mock.AnythingOfType("string")).RunAndReturn(func(name string) error {
					generatedFilename = name + ".jpg" // mimick ext
					return nil
				})

				f.file.EXPECT().Name().RunAndReturn(func() string {
					return generatedFilename
				})

				f.file.EXPECT().ContentType().Return("image/jpg")

				f.repo.EXPECT().
					Update(ctx, mock.MatchedBy(func(u *user.User) bool {
						return u.ProfileImage.Get() == generatedFilename
					})).
					Return(nil)

				f.storage.EXPECT().
					Put(
						ctx,
						mock.MatchedBy(func(path string) bool {
							return path == buildImagePath(generatedFilename)
						}),
						f.file,
						mock.AnythingOfType("string")).
					Return(nil)

				return fn(ctx)
			})
		/* ======================= end internal op mock sequence ======================== */

		// Execute
		err := f.service.ChangeProfilePicture(ctx, existingUser.ID, f.file)
		assert.NoError(t, err)
	})

	t.Run("Success with cleanup old image", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)

		existingUser := &user.User{
			ID:           uuid.New(),
			Name:         "John Doe",
			Email:        "john@example.com",
			Password:     "hashedpassword",
			Phone:        nullable.New("123456789", false),
			ProfileImage: nullable.New("old-image.jpg", false),
		}
		prevImageName := existingUser.ProfileImage.Get()

		/* ========================= internal op mock sequence ========================= */
		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, existingUser.ID).Return(existingUser, nil)

				var generatedFilename string
				f.file.EXPECT().SetName(mock.AnythingOfType("string")).RunAndReturn(func(name string) error {
					generatedFilename = name + ".jpg" // mimick ext
					return nil
				})

				f.file.EXPECT().Name().RunAndReturn(func() string {
					return generatedFilename
				})

				f.file.EXPECT().ContentType().Return("image/jpg")

				f.repo.EXPECT().
					Update(ctx, mock.MatchedBy(func(u *user.User) bool {
						return u.ProfileImage.Get() == generatedFilename
					})).
					Return(nil)

				f.storage.EXPECT().
					Put(
						ctx,
						mock.MatchedBy(func(path string) bool {
							return path == buildImagePath(generatedFilename)
						}),
						f.file,
						mock.AnythingOfType("string")).
					Return(nil)

				return fn(ctx)
			})

		// REASON: prevImagePath != ""
		f.storage.EXPECT().
			Delete(context.Background(), mock.MatchedBy(func(path string) bool {
				return path == buildImagePath(prevImageName)
			})).
			Return(nil).
			Maybe() // Maybe because its run inside goroutine (async)
		/* ======================= end internal op mock sequence ======================== */

		// Execute
		err := f.service.ChangeProfilePicture(ctx, existingUser.ID, f.file)
		assert.NoError(t, err)
	})

	t.Run("Error user not found", func(t *testing.T) {
		ctx := context.Background()
		f := setupTestFixture(t)
		userID := uuid.New()

		/* ========================= internal op mock sequence ========================= */
		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, userID).Return(nil, domain.ErrNotFound)
				return fn(ctx)
			})
		/* ======================= end internal op mock sequence ======================== */

		err := f.service.ChangeProfilePicture(ctx, userID, f.file)
		assert.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

type testFixtures struct {
	transactor *sharedMocks.Transactor
	file       *sharedMocks.File
	storage    *sharedMocks.Storage
	repo       *mocks.Repository
	service    *user.Service
}

func setupTestFixture(t *testing.T) *testFixtures {
	tr := sharedMocks.NewTransactor(t)
	repo := mocks.NewRepository(t)
	storage := sharedMocks.NewStorage(t)
	file := sharedMocks.NewFile(t)

	t.Cleanup(func() {
		tr.AssertExpectations(t)
		repo.AssertExpectations(t)
		storage.AssertExpectations(t)
		file.AssertExpectations(t)
	})

	svc := user.NewService(tr, repo, storage)

	return &testFixtures{
		transactor: tr,
		repo:       repo,
		storage:    storage,
		file:       file,
		service:    svc,
	}
}
