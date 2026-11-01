package app

import (
	"net/http"
	"strings"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
)

// routeGroup groups registration and middleware while ServeMux handles routing.
// Request paths remain intact, including ServeMux wildcard PathValue values.
type routeGroup struct {
	mux         *http.ServeMux
	prefix      string
	middlewares []middleware.Middleware
}
type routeGroups struct{ Admin, Karyawan *routeGroup }

func newRouteGroups(mux *http.ServeMux) routeGroups {
	api := &routeGroup{mux: mux, prefix: "/api/v1"}
	return routeGroups{Admin: api.subgroup("/admin"), Karyawan: api.subgroup("/karyawan")}
}
func (g *routeGroup) subgroup(prefix string) *routeGroup {
	return &routeGroup{mux: g.mux, prefix: g.prefix + prefix, middlewares: g.middlewares}
}
func (g *routeGroup) Group(prefix string, register func(*routeGroup)) {
	register(g.subgroup(prefix))
}
func (g *routeGroup) With(guards ...middleware.Middleware) *routeGroup {
	group := *g
	group.middlewares = append(append([]middleware.Middleware{}, g.middlewares...), guards...)
	return &group
}
func (g *routeGroup) handle(method, path string, h http.HandlerFunc) {
	fullPath := strings.TrimSuffix(g.prefix+path, "/")
	if fullPath == "" {
		fullPath = "/"
	}
	g.mux.Handle(method+" "+fullPath, middleware.Chain(h, g.middlewares...))
}
func (g *routeGroup) Get(path string, h http.HandlerFunc)  { g.handle(http.MethodGet, path, h) }
func (g *routeGroup) Post(path string, h http.HandlerFunc) { g.handle(http.MethodPost, path, h) }

func (g *routeGroup) Put(path string, h http.HandlerFunc)    { g.handle(http.MethodPut, path, h) }
func (g *routeGroup) Delete(path string, h http.HandlerFunc) { g.handle(http.MethodDelete, path, h) }
