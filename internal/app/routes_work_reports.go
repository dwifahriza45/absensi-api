package app

import (
	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	workreports "github.com/haidarrasyad/absensi-api/internal/domain/work_reports"
)

func registerWorkreportsRoutes(routes routeGroups, h *workreports.Handler, g middleware.Guards) {
	routes.Karyawan.With(g.Employee).Group("/work-reports", func(r *routeGroup) {
		r.With(g.Upload).Post("/", h.Submit)
	})
}
