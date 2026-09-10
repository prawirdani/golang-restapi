package http

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

func Authenticator(jwtSecret string) fiber.Handler {
	return func(c fiber.Ctx) error {
		tokenStr := c.Cookies(AccessTokenCookie)

		// If token doesn't exist in cookie, retrieve from Authorization header
		if tokenStr == "" {
			authHeader := c.Get("Authorization")
			if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
				tokenStr = after
			}
		}

		// If missing, return unauthorized error
		if tokenStr == "" {
			return ErrReqUnauthorized
		}

		// Validate token
		claims, err := auth.VerifyAccessToken(jwtSecret, tokenStr)
		if err != nil {
			return err
		}

		// Inject actor context
		uid := claims.UserID
		ctx := rbac.WithContext(c.Context(), rbac.Context{
			Actor: rbac.Actor{
				UserID: &uid,
				Role:   claims.Role,
			},
			SessionID: claims.SessionID,
		})

		// Inject user and session id to logger context
		ctx = log.WithContext(
			ctx,
			log.Group(
				"auth",
				"uid", claims.UserID,
				"sid", claims.SessionID,
			),
		)

		c.SetContext(ctx)
		return c.Next()
	}
}

func RateLimit(max int, exp time.Duration) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: exp,
		LimitReached: func(c fiber.Ctx) error {
			return ErrRateLimit
		},
	})
}

// RequestLoggerContext retrieve request_id from fiber context and inject it to app logger context
func RequestLoggerContext() fiber.Handler {
	return func(c fiber.Ctx) error {
		id := requestid.FromContext(c)

		ctx := log.WithContext(c.Context(), "request_id", id)
		c.SetContext(ctx)

		return c.Next()
	}
}

func RequestMeta() fiber.Handler {
	return func(c fiber.Ctx) error {
		meta := audit.RequestMeta{
			IP:        c.IP(),
			UserAgent: c.UserAgent(),
		}
		c.SetContext(audit.WithRequestMeta(c.Context(), meta))
		return c.Next()
	}
}
