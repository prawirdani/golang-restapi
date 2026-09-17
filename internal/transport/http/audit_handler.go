package http

import (
	"github.com/gofiber/fiber/v3"
	"github.com/prawirdani/golang-restapi/internal/audit"
)

type AuditHandler struct {
	service *audit.Service
}

func NewAuditHandler(service *audit.Service) *AuditHandler {
	return &AuditHandler{
		service: service,
	}
}

func (h *AuditHandler) Routes(router fiber.Router, auth *authenticatorMiddleware) {
	router.Route("/audit", func(r fiber.Router) {
		r.Get("/", auth.Authenticate, h.list)
	})
}

func (h *AuditHandler) list(c fiber.Ctx) error {
	entries, err := h.service.List(c.Context())
	if err != nil {
		return err
	}

	return c.JSON(Body{
		Data: entries,
	})
}
