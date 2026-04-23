package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	httpx "github.com/prawirdani/golang-restapi/internal/transport/http"
	"github.com/prawirdani/golang-restapi/internal/transport/http/handler"
	"github.com/prawirdani/golang-restapi/internal/transport/http/middleware"
	"github.com/prawirdani/golang-restapi/pkg/log"
	"github.com/prawirdani/golang-restapi/pkg/metrics"
)

type Server struct {
	container *Container
	router    *chi.Mux
	metrics   *metrics.Metrics
}

// NewServer acts as a constructor, initializing the server and its dependencies.
func NewServer(container *Container) (*Server, error) {
	if container == nil {
		return nil, fmt.Errorf("container is required")
	}

	router := chi.NewRouter()
	metrics := metrics.Init(
		container.Config.App.Version,
		string(container.Config.App.Environment),
		container.Config.App.Port+1,
	)

	if container.Config.IsProduction() {
		router.Use(middleware.RequestID)
		router.Use(middleware.RateLimit(50, 1*time.Minute))
		router.Use(metrics.InstrumentHandler) // Instrument the main router
	} else {
		router.Use(middleware.ReqLogger)
	}

	// Apply common middlewares
	router.Use(middleware.MaxBodySizeMiddleware(httpx.MaxBodySize))
	router.Use(httpx.Middleware(middleware.PanicRecoverer))
	router.Use(middleware.Gzip)
	router.Use(middleware.Cors(
		container.Config.Cors.Origins,
		container.Config.Cors.Credentials,
		!container.Config.IsProduction(),
	))

	router.NotFound(httpx.Handler(func(c *httpx.Context) error {
		return httpx.ErrNotFoundHandler
	}))

	router.MethodNotAllowed(httpx.Handler(func(c *httpx.Context) error {
		return httpx.ErrMethodNotAllowedHandler
	}))

	// Health check route
	router.Get("/status", httpx.Handler(func(c *httpx.Context) error {
		return c.JSON(http.StatusOK, httpx.Body{
			Message: "services up and running",
		})
	}))

	svr := &Server{
		container: container,
		router:    router,
		metrics:   metrics,
	}

	// Setup API routes
	svr.setupHandlers()

	return svr, nil
}

func (s *Server) Start(ctx context.Context) error {
	cfg := s.container.Config
	port := cfg.App.Port

	// Metrics server
	var metricServer *http.Server
	if cfg.IsProduction() {
		metricServer = &http.Server{
			Addr:    fmt.Sprintf(":%v", port+1),
			Handler: s.metrics.ExporterHandler(),
		}

		// Start metrics server
		go func() {
			log.Info(
				fmt.Sprintf("Metrics serving on 0.0.0.0:%v/metrics", port+1),
			)
			if err := metricServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("Metrics server stopped unexpectedly", err)
			}
		}()
	}

	// API server
	apiServer := &http.Server{
		Addr:         fmt.Sprintf(":%v", port),
		Handler:      s.router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start API server
	go func() {
		log.Info(fmt.Sprintf("API server listening on 0.0.0.0:%v", port))
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("API server stopped unexpectedly", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		log.Error("Failed to shutdown API server", err)
	}

	if metricServer != nil {
		if err := metricServer.Shutdown(shutdownCtx); err != nil {
			log.Error("Failed to shutdown Metrics server", err)
		}
	}
	return nil
}

// setupHandlers initializes and registers all API handlers.
func (s *Server) setupHandlers() {
	svcs := s.container.Services

	// Initialize Handlers
	userHandler := handler.NewUserHandler(svcs.UserService)
	authHandler := handler.NewAuthHandler(s.container.Config, svcs.AuthService, svcs.UserService)
	authMiddleware := httpx.Middleware(middleware.Auth(s.container.Config.Auth.JwtSecret))

	// Register API routes
	s.router.Route("/api", func(r chi.Router) {
		RegisterAuthRoutes(r, authHandler, authMiddleware)

		r.With(authMiddleware).Route("/", func(r chi.Router) {
			RegisterUserRoutes(r, userHandler)
		})
	})
}
