package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/etag"
	"github.com/gofiber/fiber/v3/middleware/logger"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/prawirdani/golang-restapi/internal/transport/http"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/prawirdani/golang-restapi/pkg/metrics"
)

type Server struct {
	app        *fiber.App
	container  *Container
	metricsApp *fiber.App
}

// NewServer acts as a constructor, initializing the server and its dependencies.
func NewServer(container *Container, onPostShutdown func(error) error) (*Server, error) {
	if container == nil {
		return nil, fmt.Errorf("container is required")
	}

	app := http.NewRouter(container.Config)

	m := metrics.Init(container.Config.App.Version, string(container.Config.App.Environment))

	if container.Config.IsProduction() {
		app.Use(http.RateLimit(20, 1*time.Minute))
	}

	app.Use(recoverer.New())
	// Resolve the status from the error exactly as the app ErrorHandler will,
	// since Fiber assigns it only after the middleware chain unwinds.
	app.Use(m.InstrumentHandler(func(err error) int {
		return http.ParseError(err).Status()
	}))
	app.Use(logger.New())
	app.Use(requestid.New())
	app.Use(http.AuditContext())
	app.Use(compress.New())
	app.Use(etag.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     container.Config.Cors.Origins,
		AllowCredentials: container.Config.Cors.Credentials,
		AllowMethods:     []string{"OPTIONS", "HEAD", "GET", "POST", "PUT", "PATCH", "DELETE"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type"},
		ExposeHeaders: []string{
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
			"X-Request-Id",
			"Retry-After",
		},
		MaxAge: 600,
	}))

	app.Get("/healthz", func(c fiber.Ctx) error {
		log.DebugCtx(c.Context(), "Hello")
		return c.JSON(http.Body{
			Message: "services up and running",
		})
	})

	var metricsApp *fiber.App
	if container.Config.IsProduction() {
		metricsApp = fiber.New()
		metricsApp.Get("/metrics", adaptor.HTTPHandler(m.ExporterHandler()))
	}

	svr := &Server{
		container:  container,
		app:        app,
		metricsApp: metricsApp,
	}

	// Setup API routes
	svr.setupHandlers()

	// Not Found Handler
	app.Use(func(c fiber.Ctx) error {
		return http.ErrNotFoundHandler
	})

	app.Hooks().OnPostShutdown(onPostShutdown)

	return svr, nil
}

func (s *Server) Start() error {
	port := s.container.Config.App.Port
	if s.metricsApp != nil {
		go func() {
			addr := fmt.Sprintf(":%d", port+1)
			log.Info(fmt.Sprintf("Metrics serving on %s/metrics", addr))
			if err := s.metricsApp.Listen(addr); err != nil {
				log.Error("Metrics server stopped unexpectedly", err)
			}
		}()
	}

	return s.app.Listen(fmt.Sprintf("127.0.0.1:%v", port))
}

// Shutdown gracefully drains the API server until ctx is done. The deadline is
// owned by the caller (see main.go). The metrics exporter is intentionally left
// out of the drain: it is a sidecar, process exit closes its listener, and a
// stalled scrape must never delay the API's shutdown.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.app.ShutdownWithContext(ctx)
}

// setupHandlers initializes and registers all API handlers.
func (s *Server) setupHandlers() {
	svcs := s.container.Services

	// Initialize Handlers
	userHandler := http.NewUserHandler(s.container.Config, svcs.UserService)
	authHandler := http.NewAuthHandler(s.container.Config, svcs.AuthService, svcs.UserService)

	// Register API routes
	s.app.Route("/api", func(router fiber.Router) {
		authHandler.Routes(router)
		userHandler.Routes(router)
	})
}
