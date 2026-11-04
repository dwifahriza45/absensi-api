// Package app merakit semua bagian aplikasi: database, domain, route, dan server HTTP.
package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/haidarrasyad/absensi-api/internal/domain/bidang"
	"github.com/haidarrasyad/absensi-api/internal/domain/mitra"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/attendances"
	"github.com/haidarrasyad/absensi-api/internal/domain/auth"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/domain/photos"
	workreports "github.com/haidarrasyad/absensi-api/internal/domain/work_reports"
	"github.com/haidarrasyad/absensi-api/internal/pkg/config"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
	"github.com/haidarrasyad/absensi-api/internal/pkg/storage"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

// App adalah aplikasi yang siap dijalankan.
type App struct {
	server *http.Server
	db     *pgxpool.Pool
	store  storage.Managed
}

// New menyiapkan seluruh dependency dan route.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	db, err := database.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		return nil, err
	}

	if cfg.Environment == "production" {
		if err := auth.NewRepository(db).RejectDefaultPassword(ctx); err != nil {
			db.Close()
			return nil, err
		}
	}

	schedule, err := shared.NewSchedule(cfg.Schedule.Start, cfg.Schedule.End,
		cfg.Schedule.BreakStart, cfg.Schedule.BreakEnd, cfg.Schedule.PhotoSessions)
	if err != nil {
		db.Close()
		return nil, err
	}

	store, err := storage.New(cfg.Storage)
	if err != nil {
		db.Close()
		return nil, err
	}

	clock := shared.NewClock(cfg.Location)
	hasher := shared.NewPhoneHasher(cfg.PhoneHashSecret)
	tokens := token.NewManager(cfg.JWTSecret, db)

	// Repository
	photoRepo := photos.NewRepository(db)
	employeeRepo := employees.NewRepository(db)
	attendanceRepo := attendances.NewRepository(db)
	reportRepo := workreports.NewRepository(db)

	// Service
	photoSvc := photos.NewService(photoRepo, store, clock, cfg.UploadWorkers)
	employeeSvc := employees.NewService(employeeRepo)
	authSvc := auth.NewService(employeeSvc, auth.NewRepository(db), tokens, hasher, cfg.EmployeeLoginPerIP)
	coordinator := database.NewCoordinator(db)
	attendanceSvc := attendances.NewService(attendanceRepo, photoSvc, reportRepo, clock, schedule, coordinator)
	reportSvc := workreports.NewService(reportRepo, photoSvc, attendanceLookup{repo: attendanceRepo}, clock, schedule, coordinator)

	// Route
	mux := http.NewServeMux()
	routes := newRouteGroups(mux)
	guards := middleware.NewGuards(tokens)
	guards.Upload = middleware.UploadAdmission(cfg.MaxActiveUploads, 5*time.Second)
	registerAuthRoutes(routes, auth.NewHandler(authSvc, cfg.CookieSecure), guards)
	registerMitraRoutes(routes, mitra.NewHandler(mitra.NewService(mitra.NewRepository(db))), guards)
	registerBidangRoutes(routes, bidang.NewHandler(bidang.NewService(bidang.NewRepository(db))), guards)
	registerEmployeesRoutes(routes, employees.NewHandler(employeeSvc), guards)
	registerAttendancesRoutes(routes, attendances.NewHandler(attendanceSvc, attendances.NewRecap(attendanceSvc, recapEmployees(employeeRepo), photoSvc)), guards)
	registerWorkreportsRoutes(routes, workreports.NewHandler(reportSvc), guards)
	registerPhotosRoutes(routes, photos.NewHandler(photoSvc), guards)
	routes.Admin.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		httpx.OK(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           middleware.Chain(mux, middleware.Recover, middleware.Logger, middleware.SecurityHeaders, middleware.CORS(cfg.CORSOrigins)),
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    16 << 10,
		IdleTimeout:       60 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
	}

	return &App{server: server, db: db, store: store}, nil
}

// Run menjalankan server sampai ctx dibatalkan (Ctrl+C / SIGTERM), lalu berhenti dengan rapi.
func (a *App) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server berjalan", "addr", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("server: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	slog.Info("menghentikan server")
	return a.server.Shutdown(shutdownCtx)
}

// Close menutup koneksi database.
func (a *App) Close() {
	a.db.Close()
	_ = a.store.Close()
}

func recapEmployees(repo employees.Repository) attendances.RecapEmployees {
	return func(ctx context.Context, filter attendances.AdminFilter) ([]attendances.AdminEmployeeDTO, error) {
		staff, err := repo.List(ctx, employees.ListFilter{Mitra: filter.Mitra, Bidang: filter.Bidang, Query: filter.Query})
		if err != nil {
			return nil, err
		}
		result := make([]attendances.AdminEmployeeDTO, 0, len(staff))
		for _, e := range staff {
			result = append(result, attendances.AdminEmployeeDTO{ID: e.ID, EmployeeID: e.EmployeeID, Name: e.Name, Mitra: e.Mitra, Bidang: e.Bidang})
		}
		return result, nil
	}
}
