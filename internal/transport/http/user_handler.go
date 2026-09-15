package http

import (
	"github.com/gofiber/fiber/v3"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/internal/user"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

type UserHandler struct {
	userService *user.Service
}

func NewUserHandler(userService *user.Service) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

func (h *UserHandler) Routes(router fiber.Router, auth *authenticatorMiddleware) {
	router.Use(auth.Authenticate).Route("/users", func(router fiber.Router) {
		router.Put("/", h.updateUser)
		router.Delete("/profile-picture", h.deleteProfilePicture)
		router.Put("/profile-picture", h.changeProfilePicture)
	})
}

func (h *UserHandler) updateUser(c fiber.Ctx) error {
	ctx := c.Context()

	authz, err := rbac.GetContext(ctx)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get auth context", err)
		return err
	}

	var reqBody user.UpdateUserInput
	if err := BindValidateJSON(c, &reqBody); err != nil {
		return err
	}

	if err := h.userService.UpdateUser(ctx, *authz.Actor.UserID, reqBody); err != nil {
		return err
	}

	return c.JSON(&Body{
		Message: "user updated!",
	})
}

func (h *UserHandler) changeProfilePicture(c fiber.Ctx) error {
	ctx := c.Context()

	if !c.IsMultipart() {
		return ErrMultipartForm.SetMessage("request 'Content-Type' header must be multipart/form-data")
	}

	fileHeaders, err := c.FormFile(ImageFormKey)
	if err != nil {
		return ErrMultipartForm.SetMessage("profile picture is required").SetDetails(map[string]any{
			"key": ImageFormKey,
		})
	}

	file := NewParsedFile(fileHeaders)
	defer func() {
		if err := file.Close(); err != nil {
			log.ErrorCtx(ctx, "failed to close parsed file", err)
		}
	}()

	if err := ValidateFile(ctx, file, ValidationRules{
		MaxSize:      2 << 20, // 2MB,
		AllowedMIMEs: ImageMIMEs,
	}); err != nil {
		log.ErrorCtx(ctx, "Failed to validate image file", err)
		return err
	}

	authz, err := rbac.GetContext(ctx)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get auth context", err)
		return err
	}

	if err := h.userService.ChangeProfilePicture(ctx, *authz.Actor.UserID, file); err != nil {
		log.ErrorCtx(ctx, "Failed to change profile picture", err)
		return err
	}

	return c.JSON(&Body{
		Message: "profile picture updated!",
	})
}

func (h *UserHandler) deleteProfilePicture(c fiber.Ctx) error {
	ctx := c.Context()

	authz, err := rbac.GetContext(ctx)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get auth context", err)
		return err
	}
	if err := h.userService.DeleteProfilePicture(ctx, *authz.Actor.UserID); err != nil {
		return err
	}

	return c.JSON(&Body{
		Message: "profile picture deleted",
	})
}
