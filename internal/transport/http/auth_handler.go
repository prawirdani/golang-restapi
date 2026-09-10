package http

import (
	"errors"
	"net"
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

func (h *AuthHandler) Routes(router fiber.Router) {
	authenticator := Authenticator(h.cfg.Auth.JwtSecret)
	router.Route("/auth", func(authRouter fiber.Router) {
		authRouter.Post("/login", RateLimit(5, 1*time.Minute), h.login)

		authRouter.Post("/register", h.register)
		authRouter.Post("/refresh", h.refreshAccessToken)

		authRouter.Post("/password/recover", RateLimit(5, 1*time.Minute), h.recoverPassword)
		authRouter.Get("/password/recover/:token", h.getPasswordRecoveryToken)
		authRouter.Put("/password/reset", h.resetPassword)

		authRouter.Use(authenticator).Route("/", func(r fiber.Router) {
			r.Delete("/logout", h.logout)
			r.Get("/me", h.getCurrentUser)
			r.Put("/password/change", h.changePassword)
		})
	})
}

func (h *AuthHandler) register(c fiber.Ctx) error {
	ctx := c.Context()

	var reqBody user.CreateUserInput
	if err := BindValidateJSON(c, &reqBody); err != nil {
		return err
	}

	if err := h.authService.Register(ctx, reqBody); err != nil {
		log.ErrorCtx(ctx, "Failed to register user", err)
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(&Body{
		Message: "registration successful",
	})
}

func (h *AuthHandler) login(c fiber.Ctx) error {
	ctx := c.Context()

	var reqBody auth.LoginInput
	if err := BindValidateJSON(c, &reqBody); err != nil {
		return err
	}
	reqBody.Meta.UserAgent = c.UserAgent()
	reqBody.Meta.IPAddr = net.ParseIP(c.IP())

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
	meta := auth.SessionMeta{
		UserAgent: c.UserAgent(),
		IPAddr:    net.ParseIP(c.IP()),
	}

	tokens, err := h.authService.RefreshAccessToken(ctx, refreshToken, meta)
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
