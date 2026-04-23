package handler

import (
	"net/http"

	"github.com/prawirdani/golang-restapi/internal/domain/auth"
	"github.com/prawirdani/golang-restapi/internal/domain/user"
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

func (h *UserHandler) ChangeProfilePicture(c *httpx.Context) error {
	if err := c.EnsureMultipartForm(); err != nil {
		return err
	}
	defer c.CleanupMultipart()

	fh, err := c.FormFile(httpx.ImageFormKey)
	if err != nil {
		if httpx.IsMissingFileError(err) {
			return httpx.ErrMultipartForm.SetDetails(map[string]any{
				"key":     httpx.ImageFormKey,
				"message": "profile image is required",
			})
		}
		log.ErrorCtx(c.Context(), "Failed to parse profile image form file", err)
		return err
	}

	file := httpx.NewParsedFile(fh)
	defer file.Close()

	if err := httpx.ValidateFile(c.Context(), file, httpx.ValidationRules{
		MaxSize:      2 << 20, // 2MB,
		AllowedMIMEs: httpx.ImageMIMEs,
	}); err != nil {
		return err
	}

	claims, err := auth.GetAccessTokenCtx(c.Context())
	if err != nil {
		return err
	}

	if err := h.userService.ChangeProfilePicture(c.Context(), claims.UserID, file); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, &httpx.Body{
		Message: "profile picture updated!",
	})
}
