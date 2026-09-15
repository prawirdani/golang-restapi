package http

import (
	"github.com/gofiber/fiber/v3"
	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/internal/audit"
)

type AuditHandler struct {
	cfg     *config.Config
	service *audit.Service
}

func NewAuditHandler(cfg *config.Config, service *audit.Service) *AuditHandler {
	return &AuditHandler{
		cfg:     cfg,
		service: service,
	}
}

func (h *AuditHandler) Routes(router fiber.Router) {
	authenticator := Authenticator(h.cfg.Auth.JwtSecret)

	router.Route("/audit", func(r fiber.Router) {
		r.Get("/", authenticator, h.list)
	})
}

func (h *AuditHandler) list(c fiber.Ctx) error {
	entries, err := h.service.List(c.Context())
	if err != nil {
		return err
	}

	return c.JSON(&Body{
		Data: entries,
	})
}
