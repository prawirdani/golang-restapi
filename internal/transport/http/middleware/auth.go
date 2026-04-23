package middleware

import (
	"strings"

	"github.com/prawirdani/golang-restapi/internal/domain/auth"
	"github.com/prawirdani/golang-restapi/internal/transport/http"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

func Auth(jwtSecret string) func(next http.Func) http.Func {
	return func(next http.Func) http.Func {
		return func(c *http.Context) error {
			var tokenStr string

			// Try from cookie
			if cookie, err := c.GetCookie(http.AccessTokenCookie); err == nil {
				tokenStr = cookie.Value
			}

			// If token doesn't exist in cookie, retrieve from Authorization header
			if tokenStr == "" {
				authHeader := c.Get("Authorization")
				if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
					tokenStr = after
				}
			}

			// If missing, return unauthorized error
			if tokenStr == "" {
				return http.ErrReqUnauthorized
			}

			// Validate token
			claims, err := auth.VerifyAccessToken(jwtSecret, tokenStr)
			if err != nil {
				return err
			}

			// Inject access token claims into request context
			ctx := auth.SetAccessTokenCtx(c.Context(), claims)
			// Inject user and session id to logger context
			ctx = log.WithContext(
				ctx,
				log.Group("auth",
					"uid", claims.UserID,
					"sid", claims.SessionID,
				),
			)
			c = c.WithContext(ctx)

			return next(c)
		}
	}
}
