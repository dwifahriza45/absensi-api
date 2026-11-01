package app

import (
	"time"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/mitra"
)

func registerMitraRoutes(routes routeGroups, h *mitra.Handler, g middleware.Guards) {
	routes.Admin.With(g.Admin).Group("/mitra", func(r *routeGroup) {
		r.Get("/", h.List)
		r.Get("/export", h.Export)
		r.Get("/import/template", h.Template)
		r.With(g.Upload, middleware.UploadAdmission(2, 5*time.Second)).Post("/import", h.Import)
		r.Post("/", h.Create)
		r.Get("/{mitra_id}", h.Detail)
		r.Put("/{mitra_id}", h.Update)
		r.Delete("/{mitra_id}", h.Delete)
	})
}
