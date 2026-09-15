package http

import (
	"net"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

type authenticatorMiddleware struct {
	jwtSecret string
}

func NewAuthenticatorMiddleware(jwtSecret string) *authenticatorMiddleware {
	return &authenticatorMiddleware{
		jwtSecret: jwtSecret,
	}
}

func (am *authenticatorMiddleware) Authenticate(c fiber.Ctx) error {
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
	claims, err := auth.VerifyAccessToken(am.jwtSecret, tokenStr)
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

func RateLimit(max int, exp time.Duration) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: exp,
		LimitReached: func(c fiber.Ctx) error {
			return ErrRateLimit
		},
	})
}

// SecurityHeaders applies Fiber's helmet middleware with an API-appropriate
// policy. HSTS is configured only in production; helmet additionally emits it
// only on secure (TLS or proxy-forwarded) requests.
func SecurityHeaders(isProduction bool) fiber.Handler {
	hstsMaxAge := 0
	if isProduction {
		hstsMaxAge = 365 * 24 * 60 * 60 // 1 year
	}

	return helmet.New(helmet.Config{
		XFrameOptions:         "DENY",
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		ContentSecurityPolicy: "default-src 'none'; frame-ancestors 'none'",
		PermissionPolicy:      "geolocation=(), microphone=(), camera=()",
		HSTSMaxAge:            hstsMaxAge,
	})
}

func AuditContext() fiber.Handler {
	return func(c fiber.Ctx) error {
		id := requestid.FromContext(c)
		ctx := audit.WithContext(
			c.Context(), audit.Context{
				IP:        net.ParseIP(c.IP()),
				RequestID: id,
				UserAgent: c.UserAgent(),
			},
		)

		// injecting request id to log context
		ctx = log.WithContext(ctx, "request_id", id)
		c.SetContext(ctx)

		return c.Next()
	}
}
