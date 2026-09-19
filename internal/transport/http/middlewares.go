package http

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/prawirdani/golang-restapi/internal/auth"
	"github.com/prawirdani/golang-restapi/internal/ports/revocation"
	"github.com/prawirdani/golang-restapi/internal/rbac"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/prawirdani/golang-restapi/pkg/metrics"
)

// revocationCheckTimeout bounds the access-token revocation lookup so a slow or
// stalled store cannot hold the request open. A timeout is treated like any
// other check error (fail-open by default, fail-closed when configured).
const revocationCheckTimeout = 300 * time.Millisecond

type authenticatorMiddleware struct {
	jwtSecret  string
	checker    revocation.Checker
	metrics    *metrics.Metrics
	failClosed bool
}

func NewAuthenticatorMiddleware(
	jwtSecret string,
	checker revocation.Checker,
	m *metrics.Metrics,
	failClosed bool,
) *authenticatorMiddleware {
	return &authenticatorMiddleware{
		jwtSecret:  jwtSecret,
		checker:    checker,
		metrics:    m,
		failClosed: failClosed,
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

	// Validate token. The revocation check below must never run before this
	// succeeds: unverified claims are attacker-controlled.
	claims, err := auth.VerifyAccessToken(am.jwtSecret, tokenStr)
	if err != nil {
		return err
	}

	// Revocation check. One store call (a single MGET); the result deliberately
	// collapses revoked and invalid into the same 401 so the response does not
	// reveal which. On a store error, fail open by default (a short access-token
	// TTL bounds exposure) unless failClosed is configured.
	revCtx, cancel := context.WithTimeout(c.Context(), revocationCheckTimeout)
	revoked, err := am.checker.IsRevoked(revCtx, claims.UserID, claims.SessionID, claims.IssuedAt.Time)
	cancel()

	if err != nil {
		log.ErrorCtx(c.Context(), "Failed to check access token revocation", err)
		if am.metrics != nil {
			am.metrics.RevocationCheckErrors.Inc()
		}
		if am.failClosed {
			return auth.ErrSessionInvalid
		}
	} else if revoked {
		return auth.ErrSessionInvalid
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
