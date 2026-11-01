package config

import (
	"strings"
	"testing"
)

func TestProductionRejectsUnsafeSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
	t.Setenv("PHONE_HASH_SECRET", strings.Repeat("y", 32))
	t.Setenv("APP_ENV", "production")
	t.Setenv("COOKIE_SECURE", "false")
	if _, err := Load(); err == nil {
		t.Fatal("insecure production cookies accepted")
	}
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("PHONE_HASH_SECRET", "ganti-dengan-teks-acak-minimal-32-karakter")
	if _, err := Load(); err == nil {
		t.Fatal("example secret accepted")
	}
	t.Setenv("PHONE_HASH_SECRET", strings.Repeat("y", 32))
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COOKIE_SECURE", "typo")
	if _, err := Load(); err == nil {
		t.Fatal("invalid boolean accepted")
	}
}

func TestCapacityConfigurationBounds(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
	t.Setenv("PHONE_HASH_SECRET", strings.Repeat("y", 32))
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("DB_MAX_CONNS", "32")
	t.Setenv("DB_MIN_CONNS", "4")
	t.Setenv("UPLOAD_WORKERS", "4")
	t.Setenv("MAX_ACTIVE_UPLOADS", "8")
	t.Setenv("EMPLOYEE_LOGIN_PER_IP", "3000")
	cfg, err := Load()
	if err != nil || cfg.DBMaxConns != 32 || cfg.UploadWorkers != 4 || cfg.EmployeeLoginPerIP != 3000 {
		t.Fatalf("config: %+v %v", cfg, err)
	}
	t.Setenv("DB_MIN_CONNS", "33")
	if _, err = Load(); err == nil {
		t.Fatal("min connections above max accepted")
	}
	t.Setenv("DB_MIN_CONNS", "4")
	t.Setenv("MAX_ACTIVE_UPLOADS", "3")
	if _, err = Load(); err == nil {
		t.Fatal("admission below worker count accepted")
	}
}
