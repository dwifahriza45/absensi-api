package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/attendances"
	"github.com/haidarrasyad/absensi-api/internal/domain/auth"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/domain/photos"
	workreports "github.com/haidarrasyad/absensi-api/internal/domain/work_reports"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
)

func TestProtectedRoutesRequireSession(t *testing.T) {
	mux := http.NewServeMux()
	routes := newRouteGroups(mux)
	guards := middleware.NewGuards(token.NewManager("test-secret", nil))
	registerAuthRoutes(routes, auth.NewHandler(nil, false), guards)
	registerEmployeesRoutes(routes, employees.NewHandler(nil), guards)
	registerAttendancesRoutes(routes, attendances.NewHandler(nil), guards)
	registerPhotosRoutes(routes, photos.NewHandler(nil), guards)
	registerWorkreportsRoutes(routes, workreports.NewHandler(nil), guards)
	for _, pattern := range []string{
		"GET /api/v1/admin/auth/me",
		"GET /api/v1/admin/employees",
		"GET /api/v1/admin/employees/filter-options",
		"POST /api/v1/admin/employees/import",
		"GET /api/v1/admin/attendances",
		"GET /api/v1/admin/attendances/version",
		"GET /api/v1/admin/photos/00000000-0000-0000-0000-000000000001",
		"GET /api/v1/karyawan/auth/me",
		"GET /api/v1/karyawan/attendances/today",
		"POST /api/v1/karyawan/attendances/check-in",
		"POST /api/v1/karyawan/attendances/check-out",
		"POST /api/v1/karyawan/attendances/early-leave",
		"POST /api/v1/karyawan/work-reports",
	} {
		t.Run(pattern, func(t *testing.T) {
			// Every listed handler has nil dependencies: it must never be reached anonymously.
			var method, path string
			for i, c := range pattern {
				if c == ' ' {
					method = pattern[:i]
					path = pattern[i+1:]
					break
				}
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest(method, path, nil))
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGroupedAuthKeepsLoginPublicAndMeRoleSpecific(t *testing.T) {
	mux := http.NewServeMux()
	guard := func(role string) middleware.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Header().Set("X-Test-Role", role); w.WriteHeader(204) })
		}
	}
	g := middleware.Guards{Admin: guard("admin"), Employee: guard("karyawan")}
	registerAuthRoutes(newRouteGroups(mux), auth.NewHandler(nil, false), g)
	for _, role := range []string{"admin", "karyawan"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/"+role+"/auth/me", nil))
		if w.Code != 204 || w.Header().Get("X-Test-Role") != role {
			t.Fatalf("incorrect me guard for %s: %d %s", role, w.Code, w.Header().Get("X-Test-Role"))
		}
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/"+role+"/auth/login", strings.NewReader("{")))
		if w.Code != 400 || w.Header().Get("X-Test-Role") != "" {
			t.Fatalf("login incorrectly guarded for %s", role)
		}
		w = httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/"+role+"/auth/refresh", nil))
		if w.Code != 401 || !strings.Contains(w.Body.String(), "Cookie refresh tidak ada") || w.Header().Get("X-Test-Role") != "" {
			t.Fatalf("refresh incorrectly guarded for %s: %s", role, w.Body.String())
		}
	}
}

func TestGroupsPreserveWildcardMethodAndMiddlewareOrder(t *testing.T) {
	mux := http.NewServeMux()
	routes := newRouteGroups(mux)
	order := ""
	wrap := func(name string) middleware.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { order += name + ":"; next.ServeHTTP(w, r) })
		}
	}
	routes.Admin.With(wrap("admin")).Group("/photos", func(r *routeGroup) {
		r.With(wrap("inner")).Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
			order += "handler"
			_, _ = w.Write([]byte(r.PathValue("id")))
		})
	})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/admin/photos/PHO-20261031-000001", nil))
	if w.Code != 200 || w.Body.String() != "PHO-20261031-000001" || order != "admin:inner:handler" {
		t.Fatalf("group routing changed: %d %s %s", w.Code, w.Body.String(), order)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/admin/photos/PHO-20261031-000001", nil))
	if w.Code != 405 {
		t.Fatalf("wrong method accepted: %d", w.Code)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/karyawan/photos/PHO-20261031-000001", nil))
	if w.Code != 404 {
		t.Fatalf("admin route leaked into karyawan group: %d", w.Code)
	}
}
