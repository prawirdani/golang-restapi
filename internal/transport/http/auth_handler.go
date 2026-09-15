package http

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/pkg/log"

	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/internal/user"
)

type AuthHandler struct {
	cfg         *config.Config
	authService *auth.Service
	userService *user.Service
}

func NewAuthHandler(
	cfg *config.Config,
	authService *auth.Service,
	userService *user.Service,
) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		userService: userService,
		cfg:         cfg,
	}
}

func (h *AuthHandler) Routes(router fiber.Router, auth *authenticatorMiddleware) {
	router.Route("/auth", func(authRouter fiber.Router) {
		authRouter.Post("/login", RateLimit(5, 1*time.Minute), h.login)

		if h.cfg.App.InternalMode {
			authRouter.Post("/register", auth.Authenticate, h.register)
		} else {
			authRouter.Post("/register", h.register)
		}
		authRouter.Post("/register/complete", h.completeRegistration)
		authRouter.Get("/register/:token", h.getRegistrationToken)

		authRouter.Post("/refresh", h.refreshAccessToken)

		authRouter.Post("/password/recover", RateLimit(5, 1*time.Minute), h.recoverPassword)
		authRouter.Get("/password/recover/:token", h.getPasswordRecoveryToken)
		authRouter.Put("/password/reset", h.resetPassword)

		authRouter.Use(auth.Authenticate).Route("/", func(r fiber.Router) {
			r.Delete("/logout", h.logout)
			r.Get("/me", h.getCurrentUser)
			r.Put("/password/change", h.changePassword)
			r.Get("/permissions", h.listPermission)
		})
	})
}

// register starts the invitation flow (public, or admin/system only when
// APP_INTERNAL_MODE is enabled). No account exists until completion.
func (h *AuthHandler) register(c fiber.Ctx) error {
	ctx := c.Context()

	var reqBody auth.RegisterInput
	if err := BindValidateJSON(c, &reqBody); err != nil {
		return err
	}

	if err := h.authService.Register(ctx, reqBody); err != nil {
		log.ErrorCtx(ctx, "Failed to register user", err)
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(&Body{
		Message: "registration successful, check your email",
	})
}

// completeRegistration consumes the emailed token and creates the account
// with the chosen password.
func (h *AuthHandler) completeRegistration(c fiber.Ctx) error {
	ctx := c.Context()

	var reqBody auth.CompleteRegistrationInput
	if err := BindValidateJSON(c, &reqBody); err != nil {
		return err
	}

	if err := h.authService.CompleteRegistration(ctx, reqBody); err != nil {
		log.ErrorCtx(ctx, "Failed to complete user registration", err)
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(&Body{
		Message: "registration completed",
	})
}

// getRegistrationToken exposes a registration token's status so the completion
// form can show whether the link is still usable.
func (h *AuthHandler) getRegistrationToken(c fiber.Ctx) error {
	ctx := c.Context()

	rawToken := c.Params("token")

	token, err := h.authService.GetRegistrationToken(ctx, rawToken)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get registration token", err)
		return err
	}

	return c.JSON(&Body{
		Data: map[string]any{
			"expires_at": token.ExpiresAt,
			"used_at":    token.UsedAt,
		},
	})
}

func (h *AuthHandler) login(c fiber.Ctx) error {
	ctx := c.Context()

	var reqBody auth.LoginInput
	if err := BindValidateJSON(c, &reqBody); err != nil {
		return err
	}

	tokens, err := h.authService.Login(ctx, reqBody)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to login", err)
		return err
	}

	if err := h.setTokenCookies(c, tokens); err != nil {
		return err
	}

	return c.JSON(&Body{
		Data: tokens,
	})
}

func (h *AuthHandler) getCurrentUser(c fiber.Ctx) error {
	ctx := c.Context()

	aCtx, err := rbac.GetContext(ctx)
	if err != nil {
		return err
	}

	usr, err := h.userService.GetUserByID(ctx, *aCtx.Actor.UserID)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get current user", err)
		return err
	}

	return c.JSON(&Body{
		Data: usr,
	})
}

func (h *AuthHandler) refreshAccessToken(c fiber.Ctx) error {
	ctx := c.Context()

	refreshToken := c.Cookies(RefreshTokenCookie)

	// If token doesn't exist in cookie, retrieve from Authorization header
	if refreshToken == "" {
		authHeader := c.Get("Authorization")
		if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
			refreshToken = after
		}
	}

	// If token is still empty, return an error
	if refreshToken == "" {
		return ErrReqUnauthorized
	}

	tokens, err := h.authService.RefreshAccessToken(ctx, refreshToken)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to refresh access token", err)
		return err
	}

	if err := h.setTokenCookies(c, tokens); err != nil {
		return err
	}

	return c.JSON(&Body{
		Data: tokens,
	})
}

func (h *AuthHandler) logout(c fiber.Ctx) error {
	ctx := c.Context()

	aCtx, _ := rbac.GetContext(ctx)
	if aCtx != nil {
		if err := h.authService.Logout(ctx, aCtx.SessionID); err != nil {
			log.ErrorCtx(ctx, "Failed to logout", err)
		}
	}

	h.removeTokenCookies(c)

	return c.JSON(&Body{
		Message: "logged out",
	})
}

func (h *AuthHandler) recoverPassword(c fiber.Ctx) error {
	ctx := c.Context()

	var reqBody auth.RecoverPasswordInput
	if err := BindValidateJSON(c, &reqBody); err != nil {
		return err
	}

	res, err := h.authService.RecoverPassword(ctx, reqBody)
	c.Set("Retry-After", strconv.FormatInt(res.RetryAfter.UTC().Unix(), 10))

	if err != nil {
		log.ErrorCtx(ctx, "Failed to recover password", err)
		return err
	}

	return c.JSON(&Body{
		Message: "password recovery email has been sent",
	})
}

func (h *AuthHandler) getPasswordRecoveryToken(c fiber.Ctx) error {
	ctx := c.Context()
	token := c.Params("token")

	tokenObj, err := h.authService.GetPasswordRecoveryToken(ctx, token)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get password recovery token", err)
		return err
	}

	return c.JSON(&Body{
		Data: map[string]any{
			"expires_at": tokenObj.ExpiresAt,
			"used_at":    tokenObj.UsedAt,
		},
	})
}

func (h *AuthHandler) resetPassword(c fiber.Ctx) error {
	ctx := c.Context()

	var reqBody auth.ResetPasswordInput
	if err := BindValidateJSON(c, &reqBody); err != nil {
		return err
	}

	if err := h.authService.ResetPassword(ctx, reqBody); err != nil {
		log.ErrorCtx(ctx, "Failed to reset password", err)
		return err
	}

	return c.JSON(&Body{
		Message: "Password has been reset successfully!",
	})
}

func (h *AuthHandler) changePassword(c fiber.Ctx) error {
	ctx := c.Context()

	var reqBody auth.ChangePasswordInput
	if err := BindValidateJSON(c, &reqBody); err != nil {
		return err
	}

	aCtx, err := rbac.GetContext(ctx)
	if err != nil {
		return err
	}

	if err := h.authService.ChangePassword(ctx, *aCtx.Actor.UserID, reqBody); err != nil {
		log.ErrorCtx(ctx, "Failed to change password", err)
		return err
	}

	return c.JSON(&Body{
		Message: "Password has been changed successfully!",
	})
}

func (h *AuthHandler) listPermission(c fiber.Ctx) error {
	ctx := c.Context()
	perms, err := h.authService.ListPermission(ctx)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get permissions", err)
		return err
	}
	// sort for consistent etag
	slices.Sort(perms)

	return c.JSON(&Body{
		Data: map[string]any{
			"permissions": perms,
		},
	})
}

func (h *AuthHandler) setTokenCookies(c fiber.Ctx, tokenPair *auth.TokenPair) error {
	if tokenPair == nil {
		return errors.New("token pair is nil")
	}

	now := time.Now()
	base := fiber.Cookie{
		HTTPOnly: true,
		Path:     "/", // Domain: ".example.com",
		Secure:   h.cfg.IsProduction(),
		SameSite: "Lax",
	}

	accessTokenCookie := base
	accessTokenCookie.Name = AccessTokenCookie
	accessTokenCookie.Value = tokenPair.AccessToken
	accessTokenCookie.Expires = now.Add(h.cfg.Auth.JwtTTL)
	c.Cookie(&accessTokenCookie)

	refreshTokenCookie := base
	refreshTokenCookie.Name = RefreshTokenCookie
	refreshTokenCookie.Value = tokenPair.RefreshToken
	refreshTokenCookie.Expires = now.Add(h.cfg.Auth.SessionTTL)
	c.Cookie(&refreshTokenCookie)

	return nil
}

func (h *AuthHandler) removeTokenCookies(c fiber.Ctx) {
	accessTokenCookie := &fiber.Cookie{
		Name:     AccessTokenCookie,
		Value:    "",
		Expires:  time.Unix(0, 0),
		HTTPOnly: true,
		Secure:   h.cfg.IsProduction(),
		Path:     "/",
	}

	sessCookie := *accessTokenCookie
	sessCookie.Name = RefreshTokenCookie

	c.Cookie(accessTokenCookie)
	c.Cookie(&sessCookie)
}
