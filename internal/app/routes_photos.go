package app

import (
	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/photos"
)

func registerPhotosRoutes(routes routeGroups, h *photos.Handler, g middleware.Guards) {
	routes.Admin.With(g.Admin).Group("/photos", func(r *routeGroup) {
		r.Get("/{id}", h.Show)
	})
}
