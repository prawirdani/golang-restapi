package http

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/prawirdani/golang-restapi/config"
)

func NewRouter(cfg *config.Config) *fiber.App {
	trustedProxies := make([]string, len(cfg.Proxy.TrustedProxies))
	for i, proxy := range cfg.Proxy.TrustedProxies {
		trustedProxies[i] = proxy.String()
	}

	router := fiber.New(fiber.Config{
		EnableSplittingOnParsers: true, // Enable comma separated for multiple query param values
		TrustProxy:               true,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies: trustedProxies,
		},
		BodyLimit: MaxBodySize,
		ErrorHandler: func(c fiber.Ctx, err error) error {
			e := ParseError(err)
			err = c.Status(e.status).JSON(map[string]any{"error": e})
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("Internal server error")
			}
			return nil
		},
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})

	return router
}
