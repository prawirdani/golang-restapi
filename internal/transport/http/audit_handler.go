package http

import (
	"github.com/gofiber/fiber/v3"
	"github.com/prawirdani/golang-restapi/internal/audit"
	"github.com/prawirdani/golang-restapi/pkg/log"
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
	ctx := c.Context()
	search := new(audit.Search)
	if err := c.Bind().Query(search); err != nil {
		return err
	}

	entries, err := h.service.List(ctx, search)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to list audit logs", err)
		return err
	}

	return c.JSON(Body{
		Data: entries,
		Meta: search.Meta(),
	})
}
