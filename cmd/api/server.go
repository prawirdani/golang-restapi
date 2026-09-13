package main

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/etag"
	"github.com/gofiber/fiber/v3/middleware/logger"
	recoverer "github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/prawirdani/golang-restapi/internal/transport/http"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

type Server struct {
	app       *fiber.App
	container *Container
}

// NewServer acts as a constructor, initializing the server and its dependencies.
func NewServer(container *Container, onPostShutdown func(error) error) (*Server, error) {
	if container == nil {
		return nil, fmt.Errorf("container is required")
	}

	app := http.NewRouter(container.Config)

	if container.Config.IsProduction() {
		app.Use(http.RateLimit(20, 1*time.Minute))
	}

	app.Use(recoverer.New())
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

	svr := &Server{
		container: container,
		app:       app,
		// metrics:   metrics,
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
	return s.app.Listen(fmt.Sprintf("127.0.0.1:%v", port))
}

func (s *Server) Shutdown() error {
	return s.app.Shutdown()
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
