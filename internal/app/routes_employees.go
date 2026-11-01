package app

import (
	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
)

func registerEmployeesRoutes(routes routeGroups, h *employees.Handler, g middleware.Guards) {
	routes.Karyawan.Group("/employees", func(r *routeGroup) {
		r.Get("/search", h.Search)
	})
	routes.Admin.With(g.Admin).Group("/employees", func(r *routeGroup) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{employee_id}", h.Detail)
		r.Put("/{employee_id}", h.Update)
		r.Put("/{employee_id}/status", h.Status)
		r.Delete("/{employee_id}", h.Delete)
		r.Get("/filter-options", h.FilterOptions)
		r.Get("/export", h.Export)
		r.Get("/import/template", h.Template)
		r.Post("/import", h.ImportRows)
	})
}
