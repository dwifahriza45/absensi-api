package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // zona waktu tetap tersedia walau server tidak punya data tz

	"github.com/haidarrasyad/absensi-api/internal/app"
	"github.com/haidarrasyad/absensi-api/internal/pkg/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("konfigurasi tidak valid", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg)
	if err != nil {
		slog.Error("gagal menyiapkan aplikasi", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	if err := application.Run(ctx); err != nil {
		slog.Error("server berhenti karena error", "error", err)
		os.Exit(1)
	}
}
