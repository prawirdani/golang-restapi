package user_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	sharedMocks "github.com/prawirdani/golang-restapi/internal/testing/mocks"
	"github.com/prawirdani/golang-restapi/internal/user"
	"github.com/prawirdani/golang-restapi/internal/user/mocks"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/prawirdani/golang-restapi/pkg/nullable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func init() {
	log.SetLogger(log.EmptyLog)
}

// adminCtx returns a context carrying an admin actor so permission checks pass.
func adminCtx() context.Context {
	adminID := uuid.New()
	return rbac.WithContext(context.Background(), rbac.Context{
		Actor: rbac.Actor{UserID: &adminID, Role: rbac.RoleAdmin},
	})
}

func TestNewUserService(t *testing.T) {
	f := setupTestFixture(t)
	require.NotNil(t, f.service)
}

func TestService_GetUserByID(t *testing.T) {
	t.Run("Without profile picture", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		userID := uuid.New()
		expectedUser := &user.User{
			ID:             userID,
			Name:           "John Doe",
			Email:          "john@example.com",
			Password:       "hashedpassword",
			Phone:          nullable.New("123456789", false),
			ProfilePicture: nullable.New("", false),
		}

		f.repo.EXPECT().GetByID(ctx, userID).Return(expectedUser, nil)

		u, err := f.service.GetUserByID(ctx, userID)
		assert.NoError(t, err)
		assert.Equal(t, expectedUser, u)
		assert.False(t, u.ProfilePicture.NotNull())
	})

	t.Run("With profile picture", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		userID := uuid.New()
		expectedUser := &user.User{
			ID:             userID,
			Name:           "John Doe",
			Email:          "john@example.com",
			Password:       "hashedpassword",
			Phone:          nullable.New("123456789", false),
			ProfilePicture: nullable.New("profile.jpg", false),
		}

		f.repo.EXPECT().GetByID(ctx, userID).Return(expectedUser, nil)

		result, err := f.service.GetUserByID(ctx, userID)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestService_GetUserByEmail(t *testing.T) {
	t.Run("Without profile picture", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		email := "john@example.com"
		expectedUser := &user.User{
			ID:             uuid.New(),
			Name:           "John Doe",
			Email:          email,
			Password:       "hashedpassword",
			Phone:          nullable.New("123456789", false),
			ProfilePicture: nullable.New("", false),
		}

		f.repo.EXPECT().GetByEmail(ctx, email).Return(expectedUser, nil)

		result, err := f.service.GetUserByEmail(ctx, email)
		assert.NoError(t, err)
		assert.Equal(t, expectedUser, result)
		assert.False(t, result.ProfilePicture.NotNull())
	})

	t.Run("With profile picture", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		email := "john@example.com"
		expectedUser := &user.User{
			ID:             uuid.New(),
			Name:           "John Doe",
			Email:          email,
			Password:       "hashedpassword",
			Phone:          nullable.New("123456789", false),
			ProfilePicture: nullable.New("profile.jpg", false),
		}

		f.repo.EXPECT().GetByEmail(ctx, email).Return(expectedUser, nil)

		result, err := f.service.GetUserByEmail(ctx, email)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}

func TestService_ChangeProfilePicture(t *testing.T) {
	t.Run("Success without existing profile picture", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		existingUser := &user.User{
			ID:             uuid.New(),
			Name:           "John Doe",
			Email:          "john@example.com",
			Password:       "hashedpassword",
			Phone:          nullable.New("123456789", false),
			ProfilePicture: nullable.New("", false),
		}

		f.file.EXPECT().SetName(mock.AnythingOfType("string")).Return(nil)
		f.file.EXPECT().Name().Return("new-image.jpg")
		f.file.EXPECT().ContentType().Return("image/jpeg")

		f.storage.EXPECT().
			Put(ctx, mock.MatchedBy(func(path string) bool {
				return path == "profiles/new-image.jpg"
			}), f.file, "image/jpeg").
			Return(nil)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, existingUser.ID).Return(existingUser, nil)
				f.repo.EXPECT().Update(ctx, mock.MatchedBy(func(u *user.User) bool {
					return u.ID == existingUser.ID && u.ProfilePicture.Get() == "new-image.jpg"
				})).Return(nil)
				f.audit.EXPECT().Record(ctx, mock.AnythingOfType("audit.Entry")).Return(nil)
				return fn(ctx)
			})

		err := f.service.ChangeProfilePicture(ctx, existingUser.ID, f.file)
		assert.NoError(t, err)
	})

	t.Run("Success with existing profile picture", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		existingUser := &user.User{
			ID:             uuid.New(),
			Name:           "John Doe",
			Email:          "john@example.com",
			Password:       "hashedpassword",
			Phone:          nullable.New("123456789", false),
			ProfilePicture: nullable.New("old-image.jpg", false),
		}

		f.file.EXPECT().SetName(mock.AnythingOfType("string")).Return(nil)
		f.file.EXPECT().Name().Return("new-image.jpg")
		f.file.EXPECT().ContentType().Return("image/jpeg")

		f.storage.EXPECT().
			Put(ctx, mock.MatchedBy(func(path string) bool {
				return path == "profiles/new-image.jpg"
			}), f.file, "image/jpeg").
			Return(nil)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, existingUser.ID).Return(existingUser, nil)
				f.repo.EXPECT().Update(ctx, mock.MatchedBy(func(u *user.User) bool {
					return u.ID == existingUser.ID && u.ProfilePicture.Get() == "new-image.jpg"
				})).Return(nil)
				f.audit.EXPECT().Record(ctx, mock.AnythingOfType("audit.Entry")).Return(nil)
				return fn(ctx)
			})

		f.storage.EXPECT().
			Delete(mock.Anything, "profiles/old-image.jpg").
			Return(nil).
			Maybe()

		err := f.service.ChangeProfilePicture(ctx, existingUser.ID, f.file)
		assert.NoError(t, err)
	})

	t.Run("Error user not found", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)
		userID := uuid.New()

		f.file.EXPECT().SetName(mock.AnythingOfType("string")).Return(nil)
		f.file.EXPECT().Name().Return("new-image.jpg")
		f.file.EXPECT().ContentType().Return("image/jpeg")

		f.storage.EXPECT().
			Put(ctx, mock.AnythingOfType("string"), f.file, "image/jpeg").
			Return(nil)

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, userID).Return(nil, apperr.ErrNotFound)
				return fn(ctx)
			})

		f.storage.EXPECT().
			Delete(mock.Anything, mock.AnythingOfType("string")).
			Return(nil).
			Maybe()

		err := f.service.ChangeProfilePicture(ctx, userID, f.file)
		assert.Error(t, err)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})

	t.Run("Error storage put fails", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		f.file.EXPECT().SetName(mock.AnythingOfType("string")).Return(nil)
		f.file.EXPECT().Name().Return("new-image.jpg")
		f.file.EXPECT().ContentType().Return("image/jpeg")

		f.storage.EXPECT().
			Put(ctx, mock.AnythingOfType("string"), f.file, "image/jpeg").
			Return(fmt.Errorf("storage unavailable"))

		err := f.service.ChangeProfilePicture(ctx, uuid.New(), f.file)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "storage unavailable")
	})

	t.Run("Error file set name fails", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		f.file.EXPECT().SetName(mock.AnythingOfType("string")).Return(fmt.Errorf("file error"))

		err := f.service.ChangeProfilePicture(ctx, uuid.New(), f.file)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "file error")
	})
}

func TestService_DeleteProfilePicture(t *testing.T) {
	t.Run("Success with existing profile picture", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		existingUser := &user.User{
			ID:             uuid.New(),
			Name:           "John Doe",
			Email:          "john@example.com",
			Password:       "hashedpassword",
			ProfilePicture: nullable.New("profile.jpg", false),
		}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, existingUser.ID).Return(existingUser, nil)
				f.repo.EXPECT().Update(ctx, mock.MatchedBy(func(u *user.User) bool {
					return u.ID == existingUser.ID && !u.ProfilePicture.NotNull()
				})).Return(nil)
				f.audit.EXPECT().Record(ctx, mock.AnythingOfType("audit.Entry")).Return(nil)
				return fn(ctx)
			})

		f.storage.EXPECT().
			Delete(mock.Anything, "profiles/profile.jpg").
			Return(nil).
			Maybe()

		err := f.service.DeleteProfilePicture(ctx, existingUser.ID)
		assert.NoError(t, err)
	})

	t.Run("Success without profile picture", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		existingUser := &user.User{
			ID:             uuid.New(),
			Name:           "John Doe",
			Email:          "john@example.com",
			Password:       "hashedpassword",
			ProfilePicture: nullable.New("", false),
		}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, existingUser.ID).Return(existingUser, nil)
				return fn(ctx)
			})

		err := f.service.DeleteProfilePicture(ctx, existingUser.ID)
		assert.NoError(t, err)
	})

	t.Run("Error user not found", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)
		userID := uuid.New()

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, userID).Return(nil, apperr.ErrNotFound)
				return fn(ctx)
			})

		err := f.service.DeleteProfilePicture(ctx, userID)
		assert.Error(t, err)
		assert.ErrorIs(t, err, apperr.ErrNotFound)
	})

	t.Run("Error update fails", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		existingUser := &user.User{
			ID:             uuid.New(),
			Name:           "John Doe",
			Email:          "john@example.com",
			Password:       "hashedpassword",
			ProfilePicture: nullable.New("profile.jpg", false),
		}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, existingUser.ID).Return(existingUser, nil)
				f.repo.EXPECT().Update(ctx, mock.AnythingOfType("*user.User")).Return(fmt.Errorf("db error"))
				return fn(ctx)
			})

		err := f.service.DeleteProfilePicture(ctx, existingUser.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "db error")
	})
}

// userCtx returns a context carrying a plain (non-privileged) user actor.
func userCtx(id uuid.UUID) context.Context {
	return rbac.WithContext(context.Background(), rbac.Context{
		Actor: rbac.Actor{UserID: &id, Role: rbac.RoleUser},
	})
}

func TestService_UpdateUser(t *testing.T) {
	t.Run("Privileged actor updates another user", func(t *testing.T) {
		ctx := adminCtx()
		f := setupTestFixture(t)

		existingUser := &user.User{
			ID:       uuid.New(),
			Name:     "John Doe",
			Email:    "john@example.com",
			Password: "hashedpassword",
			Role:     rbac.RoleUser,
		}
		input := user.UpdateUserInput{Name: "Jane Doe", Phone: "987654321", Gender: "f"}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, existingUser.ID).Return(existingUser, nil)
				f.repo.EXPECT().Update(ctx, mock.MatchedBy(func(u *user.User) bool {
					return u.ID == existingUser.ID && u.Name == "Jane Doe" &&
						u.Phone.Get() == "987654321" && u.Gender.Get() == user.Gender("F")
				})).Return(nil)
				f.audit.EXPECT().Record(ctx, mock.AnythingOfType("audit.Entry")).Return(nil)
				return fn(ctx)
			})

		err := f.service.UpdateUser(ctx, existingUser.ID, input)
		assert.NoError(t, err)
	})

	t.Run("User updates self", func(t *testing.T) {
		selfID := uuid.New()
		ctx := userCtx(selfID)
		f := setupTestFixture(t)

		existingUser := &user.User{
			ID:       selfID,
			Name:     "John Doe",
			Email:    "john@example.com",
			Password: "hashedpassword",
			Role:     rbac.RoleUser,
		}

		f.transactor.EXPECT().
			Transact(ctx, mock.AnythingOfType("func(context.Context) error")).
			RunAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
				f.repo.EXPECT().GetByID(ctx, selfID).Return(existingUser, nil)
				f.repo.EXPECT().Update(ctx, mock.AnythingOfType("*user.User")).Return(nil)
				f.audit.EXPECT().Record(ctx, mock.AnythingOfType("audit.Entry")).Return(nil)
				return fn(ctx)
			})

		err := f.service.UpdateUser(ctx, selfID, user.UpdateUserInput{Name: "Johnny"})
		assert.NoError(t, err)
	})

	t.Run("User cannot update another user", func(t *testing.T) {
		ctx := userCtx(uuid.New())
		f := setupTestFixture(t)

		// No transactor/repo expectations: authorization must reject before any
		// write path runs, so an unexpected call would fail the test.
		err := f.service.UpdateUser(ctx, uuid.New(), user.UpdateUserInput{Name: "Hacked"})
		assert.ErrorIs(t, err, rbac.ErrUnauthorizedPermission)
	})
}

type testFixtures struct {
	transactor *sharedMocks.Transactor
	file       *sharedMocks.File
	storage    *sharedMocks.Storage
	repo       *mocks.Repository
	audit      *sharedMocks.Recorder
	service    *user.Service
}

func setupTestFixture(t *testing.T) *testFixtures {
	tr := sharedMocks.NewTransactor(t)
	repo := mocks.NewRepository(t)
	storage := sharedMocks.NewStorage(t)
	file := sharedMocks.NewFile(t)
	auditRec := sharedMocks.NewRecorder(t)

	t.Cleanup(func() {
		tr.AssertExpectations(t)
		repo.AssertExpectations(t)
		storage.AssertExpectations(t)
		file.AssertExpectations(t)
		auditRec.AssertExpectations(t)
	})

	svc := user.NewService(tr, repo, storage, rbac.NewAuthorizer(), auditRec)

	return &testFixtures{
		transactor: tr,
		repo:       repo,
		storage:    storage,
		file:       file,
		audit:      auditRec,
		service:    svc,
	}
}
