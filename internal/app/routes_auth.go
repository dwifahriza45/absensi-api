package app

import (
	"net/http"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/auth"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
)

func registerAuthRoutes(routes routeGroups, h *auth.Handler, g middleware.Guards) {
	routes.Admin.Group("/auth", func(r *routeGroup) {
		r.Post("/login", h.LoginAdmin)
		r.Post("/refresh", func(w http.ResponseWriter, r *http.Request) { h.Refresh(w, r, token.RoleAdmin) })
		r.Post("/logout", func(w http.ResponseWriter, r *http.Request) { h.Logout(w, r, token.RoleAdmin) })
		r.With(g.Admin).Get("/me", h.AdminMe)
	})
	routes.Karyawan.Group("/auth", func(r *routeGroup) {
		r.Post("/login", h.LoginEmployee)
		r.Post("/refresh", func(w http.ResponseWriter, r *http.Request) { h.Refresh(w, r, token.RoleEmployee) })
		r.Post("/logout", func(w http.ResponseWriter, r *http.Request) { h.Logout(w, r, token.RoleEmployee) })
		r.With(g.Employee).Get("/me", h.Me)
	})
}
