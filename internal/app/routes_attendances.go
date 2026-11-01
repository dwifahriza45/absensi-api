package app

import (
	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/attendances"
)

func registerAttendancesRoutes(routes routeGroups, h *attendances.Handler, g middleware.Guards) {
	routes.Karyawan.With(g.Employee).Group("/attendances", func(r *routeGroup) {
		r.Get("/today", h.Today)
		r.With(g.Upload).Post("/check-in", h.CheckIn)
		r.With(g.Upload).Post("/check-out", h.CheckOut)
		r.Post("/early-leave", h.EarlyLeave)
	})
	routes.Admin.With(g.Admin).Group("/attendances", func(r *routeGroup) {
		r.Get("/", h.AdminList)
		r.Get("/version", h.Version)
		r.Get("/recap", h.Recap)
	})
}
