package app

import (
	"time"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/bidang"
)

func registerBidangRoutes(routes routeGroups, h *bidang.Handler, g middleware.Guards) {
	routes.Admin.With(g.Admin).Group("/bidang", func(r *routeGroup) {
		r.Get("/", h.List)
		r.Get("/export", h.Export)
		r.Get("/import/template", h.Template)
		r.With(g.Upload, middleware.UploadAdmission(2, 5*time.Second)).Post("/import", h.Import)
		r.Post("/", h.Create)
		r.Get("/{bidang_id}", h.Detail)
		r.Put("/{bidang_id}", h.Update)
		r.Delete("/{bidang_id}", h.Delete)
	})
}
