package handler

import (
	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/user"
	httpx "github.com/prawirdani/golang-restapi/internal/transport/http"
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

func (h *UserHandler) UpdateUser(c *httpx.Context) error {
	ctx := c.Context()

	claims, err := auth.GetAccessTokenCtx(ctx)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get access token context", err)
		return err
	}

	var reqBody user.UpdateUserInput
	if err := c.BindValidate(&reqBody); err != nil {
		return err
	}

	if err := h.userService.UpdateUser(ctx, claims.UserID, reqBody); err != nil {
		return err
	}

	return c.JSON(&httpx.Body{
		Message: "user updated!",
	})
}

func (h *UserHandler) ChangeProfilePicture(c *httpx.Context) error {
	ctx := c.Context()

	if err := c.EnsureMultipartForm(); err != nil {
		return err
	}
	defer c.CleanupMultipart()

	fh, err := c.FormFile(httpx.ImageFormKey)
	if err != nil {
		if httpx.IsMissingFileError(err) {
			return httpx.ErrMultipartForm.SetDetails(map[string]any{
				"key":     httpx.ImageFormKey,
				"message": "profile picture is required",
			})
		}
		log.ErrorCtx(ctx, "Failed to parse profile picture form file", err)
		return err
	}

	file := httpx.NewParsedFile(fh)
	defer file.Close()

	if err := httpx.ValidateFile(ctx, file, httpx.ValidationRules{
		MaxSize:      2 << 20, // 2MB,
		AllowedMIMEs: httpx.ImageMIMEs,
	}); err != nil {
		log.ErrorCtx(ctx, "Failed to validate image file", err)
		return err
	}

	claims, err := auth.GetAccessTokenCtx(ctx)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get access token context", err)
		return err
	}

	if err := h.userService.ChangeProfilePicture(ctx, claims.UserID, file); err != nil {
		log.ErrorCtx(ctx, "Failed to change profile picture", err)
		return err
	}

	return c.JSON(&httpx.Body{
		Message: "profile picture updated!",
	})
}

func (h *UserHandler) DeleteProfilePicture(c *httpx.Context) error {
	ctx := c.Context()

	claims, err := auth.GetAccessTokenCtx(ctx)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get access token context", err)
		return err
	}
	if err := h.userService.DeleteProfilePicture(ctx, claims.UserID); err != nil {
		return err
	}

	return c.JSON(&httpx.Body{
		Message: "profile picture deleted",
	})
}
