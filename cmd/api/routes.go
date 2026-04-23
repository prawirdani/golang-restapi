package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	httpx "github.com/prawirdani/golang-restapi/internal/transport/http"
	"github.com/prawirdani/golang-restapi/internal/transport/http/handler"
)

var fn = httpx.Handler

type authMiddleware = func(next http.Handler) http.Handler

func RegisterAuthRoutes(r chi.Router, h *handler.AuthHandler, authMw authMiddleware) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", fn(h.Login))
		r.Post("/register", fn(h.Register))
		r.Post("/refresh", fn(h.RefreshAccessToken))

		r.Post("/password/recover", fn(h.RecoverPassword))
		r.Get("/password/recover/{token}", fn(h.GetPasswordRecoveryToken))
		r.Post("/password/reset", fn(h.ResetPassword))

		r.With(authMw).Group(func(r chi.Router) {
			r.Delete("/logout", fn(h.Logout))
			r.Get("/me", fn(h.GetCurrentUser))
			r.Post("/password/change", fn(h.ChangePassword))
		})
	})
}

func RegisterUserRoutes(r chi.Router, h *handler.UserHandler) {
	r.Route("/users", func(r chi.Router) {
		r.Post("/profile/upload", fn(h.ChangeProfilePicture))
	})
}
