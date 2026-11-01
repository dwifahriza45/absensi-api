package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config berisi seluruh pengaturan aplikasi yang dibaca dari environment variable.
type Config struct {
	DBMaxConns         int32
	DBMinConns         int32
	EmployeeLoginPerIP int
	UploadWorkers      int
	MaxActiveUploads   int
	Environment        string
	Port               string
	DatabaseURL        string
	JWTSecret          string
	PhoneHashSecret    string
	CookieSecure       bool
	Storage            Storage
	CORSOrigins        []string
	Location           *time.Location
	Schedule           Schedule
}

// Schedule adalah jadwal kerja default dengan format jam "HH:MM".
type Schedule struct {
	Start         string
	End           string
	BreakStart    string
	BreakEnd      string
	PhotoSessions int
}

// Load membaca dan memvalidasi konfigurasi.
func Load() (Config, error) {
	loc, err := time.LoadLocation(env("APP_TIMEZONE", "Asia/Jakarta"))
	if err != nil {
		return Config{}, fmt.Errorf("APP_TIMEZONE: %w", err)
	}

	sessions, err := strconv.Atoi(env("SCHEDULE_PHOTO_SESSIONS", "3"))
	if err != nil || sessions < 1 {
		return Config{}, fmt.Errorf("SCHEDULE_PHOTO_SESSIONS harus angka minimal 1")
	}

	readInt := func(key string, fallback, min, max int) (int, error) {
		value, err := strconv.Atoi(env(key, strconv.Itoa(fallback)))
		if err != nil || value < min || value > max {
			return 0, fmt.Errorf("%s harus antara %d dan %d", key, min, max)
		}
		return value, nil
	}
	maxConns, err := readInt("DB_MAX_CONNS", 32, 4, 80)
	if err != nil {
		return Config{}, err
	}
	minConns, err := readInt("DB_MIN_CONNS", 4, 0, maxConns)
	if err != nil {
		return Config{}, err
	}
	loginLimit, err := readInt("EMPLOYEE_LOGIN_PER_IP", 3000, 100, 100000)
	if err != nil {
		return Config{}, err
	}
	workers, err := readInt("UPLOAD_WORKERS", 4, 1, 8)
	if err != nil {
		return Config{}, err
	}
	activeUploads, err := readInt("MAX_ACTIVE_UPLOADS", 8, workers, 32)
	if err != nil {
		return Config{}, err
	}
	secure, err := strconv.ParseBool(env("COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("COOKIE_SECURE harus true/false")
	}
	storageConfig, err := LoadStorage()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		DBMaxConns: int32(maxConns), DBMinConns: int32(minConns), EmployeeLoginPerIP: loginLimit, UploadWorkers: workers, MaxActiveUploads: activeUploads,
		Environment:     env("APP_ENV", "development"),
		Port:            env("APP_PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		PhoneHashSecret: os.Getenv("PHONE_HASH_SECRET"),
		CookieSecure:    secure,
		Storage:         storageConfig,
		CORSOrigins:     splitList(env("CORS_ORIGINS", "http://localhost:5173,http://localhost:5174,http://127.0.0.1:5173,http://127.0.0.1:5174")),
		Location:        loc,
		Schedule: Schedule{
			Start:         env("SCHEDULE_START", "08:00"),
			End:           env("SCHEDULE_END", "17:00"),
			BreakStart:    env("SCHEDULE_BREAK_START", "12:00"),
			BreakEnd:      env("SCHEDULE_BREAK_END", "13:00"),
			PhotoSessions: sessions,
		},
	}

	required := map[string]string{
		"DATABASE_URL":      cfg.DatabaseURL,
		"JWT_SECRET":        cfg.JWTSecret,
		"PHONE_HASH_SECRET": cfg.PhoneHashSecret,
	}
	for name, value := range required {
		if value == "" {
			return Config{}, fmt.Errorf("%s wajib diisi", name)
		}
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET minimal 32 karakter")
	}

	if cfg.Environment != "development" && cfg.Environment != "production" {
		return Config{}, fmt.Errorf("APP_ENV harus development/production")
	}
	if cfg.Environment == "production" {
		if !cfg.CookieSecure {
			return Config{}, fmt.Errorf("COOKIE_SECURE wajib true di production")
		}
		for _, secret := range []string{cfg.JWTSecret, cfg.PhoneHashSecret} {
			if len(secret) < 32 || strings.Contains(strings.ToLower(secret), "ganti-") {
				return Config{}, fmt.Errorf("secret production minimal 32 karakter dan tidak boleh memakai contoh")
			}
		}
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
