package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/prawirdani/golang-restapi/pkg/nullable"

	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/user"
	httpx "github.com/prawirdani/golang-restapi/internal/transport/http"
)

type AuthHandler struct {
	authService *auth.Service
	userService *user.Service
	cfg         *config.Config
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

func (h *AuthHandler) Register(c *httpx.Context) error {
	ctx := c.Context()

	var reqBody user.CreateUserInput
	if err := c.BindValidate(&reqBody); err != nil {
		return err
	}

	if err := h.authService.Register(ctx, reqBody); err != nil {
		log.ErrorCtx(ctx, "Failed to register user", err)
		return err
	}

	return c.Status(http.StatusCreated).JSON(&httpx.Body{
		Message: "registration successful",
	})
}

func (h *AuthHandler) Login(c *httpx.Context) error {
	ctx := c.Context()

	var reqBody auth.LoginInput
	if err := c.BindValidate(&reqBody); err != nil {
		return err
	}
	reqBody.UserAgent = c.Get("User-Agent")

	tokens, err := h.authService.Login(ctx, reqBody)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to login", err)
		return err
	}

	if err := h.setTokenCookies(c, tokens); err != nil {
		return err
	}

	return c.JSON(&httpx.Body{
		Data: tokens,
	})
}

func (h *AuthHandler) GetCurrentUser(c *httpx.Context) error {
	ctx := c.Context()

	claims, err := auth.GetAccessTokenCtx(ctx)
	if err != nil {
		return err
	}

	usr, err := h.userService.GetUserByID(ctx, claims.UserID)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get current user", err)
		return err
	}

	return c.JSON(&httpx.Body{
		Data: usr,
	})
}

func (h *AuthHandler) RefreshAccessToken(c *httpx.Context) error {
	ctx := c.Context()

	var refreshToken string

	if cookie, err := c.GetCookie(httpx.RefreshTokenCookie); err == nil {
		refreshToken = cookie.Value
	}

	// If token doesn't exist in cookie, retrieve from Authorization header
	if refreshToken == "" {
		authHeader := c.Get("Authorization")
		if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
			refreshToken = after
		}
	}

	// If token is still empty, return an error
	if refreshToken == "" {
		return httpx.ErrReqUnauthorized
	}

	tokens, err := h.authService.RefreshAccessToken(ctx, refreshToken)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to refresh access token", err)
		return err
	}

	if err := h.setTokenCookies(c, tokens); err != nil {
		return err
	}

	return c.JSON(&httpx.Body{
		Data: tokens,
	})
}

func (h *AuthHandler) Logout(c *httpx.Context) error {
	ctx := c.Context()

	authClaims, _ := auth.GetAccessTokenCtx(ctx)
	if authClaims != nil {
		if err := h.authService.Logout(ctx, authClaims.SessionID); err != nil {
			log.ErrorCtx(ctx, "Failed to logout", err)
		}
	}

	h.removeTokenCookies(c)

	return c.JSON(&httpx.Body{
		Message: "logged out",
	})
}

func (h *AuthHandler) RecoverPassword(c *httpx.Context) error {
	ctx := c.Context()

	var reqBody auth.RecoverPasswordInput
	if err := c.BindValidate(&reqBody); err != nil {
		return err
	}

	res, err := h.authService.RecoverPassword(ctx, reqBody)
	c.Set("Retry-After", strconv.FormatInt(res.RetryAfter.UTC().Unix(), 10))

	if err != nil {
		log.ErrorCtx(ctx, "Failed to recover password", err)
		return err
	}

	return c.JSON(&httpx.Body{
		Message: "password recovery email has been sent",
	})
}

func (h *AuthHandler) GetPasswordRecoveryToken(c *httpx.Context) error {
	ctx := c.Context()
	token := c.Param("token")

	tokenObj, err := h.authService.GetPasswordRecoveryToken(ctx, token)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to get password recovery token", err)
		return err
	}

	type response struct {
		ExpiresAt time.Time                    `json:"expires_at"`
		UsedAt    nullable.Nullable[time.Time] `json:"used_at"`
	}

	return c.JSON(&httpx.Body{
		Data: response{
			ExpiresAt: tokenObj.ExpiresAt,
			UsedAt:    tokenObj.UsedAt,
		},
	})
}

func (h *AuthHandler) ResetPassword(c *httpx.Context) error {
	ctx := c.Context()

	var reqBody auth.ResetPasswordInput
	if err := c.BindValidate(&reqBody); err != nil {
		return err
	}

	if err := h.authService.ResetPassword(ctx, reqBody); err != nil {
		log.ErrorCtx(ctx, "Failed to reset password", err)
		return err
	}

	return c.JSON(&httpx.Body{
		Message: "Password has been reset successfully!",
	})
}

func (h *AuthHandler) ChangePassword(c *httpx.Context) error {
	ctx := c.Context()

	var reqBody auth.ChangePasswordInput
	if err := c.BindValidate(&reqBody); err != nil {
		return err
	}

	claims, err := auth.GetAccessTokenCtx(ctx)
	if err != nil {
		return err
	}

	if err := h.authService.ChangePassword(ctx, claims.UserID, reqBody); err != nil {
		log.ErrorCtx(ctx, "Failed to change password", err)
		return err
	}

	return c.JSON(&httpx.Body{
		Message: "Password has been changed successfully!",
	})
}

func (h *AuthHandler) setTokenCookies(c *httpx.Context, tokenPair *auth.TokenPair) error {
	if tokenPair == nil {
		return errors.New("token pair is nil")
	}

	now := time.Now()
	base := http.Cookie{
		HttpOnly: true,
		Secure:   h.cfg.IsProduction(),
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		// Domain: ".example.com",
	}

	accessTokenCookie := base
	accessTokenCookie.Name = httpx.AccessTokenCookie
	accessTokenCookie.Value = tokenPair.AccessToken
	accessTokenCookie.Expires = now.Add(h.cfg.Auth.JwtTTL)
	c.SetCookie(&accessTokenCookie)

	refreshTokenCookie := base
	refreshTokenCookie.Name = httpx.RefreshTokenCookie
	refreshTokenCookie.Value = tokenPair.RefreshToken
	refreshTokenCookie.Expires = now.Add(h.cfg.Auth.SessionTTL)
	c.SetCookie(&refreshTokenCookie)

	return nil
}

func (h *AuthHandler) removeTokenCookies(c *httpx.Context) {
	accessTokenCookie := &http.Cookie{
		Name:     httpx.AccessTokenCookie,
		Value:    "",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   h.cfg.IsProduction(),
		Path:     "/",
	}

	sessCookie := *accessTokenCookie
	sessCookie.Name = httpx.RefreshTokenCookie

	c.SetCookie(accessTokenCookie)
	c.SetCookie(&sessCookie)
}
