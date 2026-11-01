package config

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Storage struct {
	Driver, Directory                string
	Endpoint, Region, Bucket, Prefix string
	AccessKeyID, SecretAccessKey     string
	ForcePathStyle                   bool
	AllowMinioHTTP                   bool
}

// LoadStorage is also used by the operator photo backup/migration tool.
// Credentials are explicit: no instance metadata or ambient AWS credential fallback.
func LoadStorage() (Storage, error) {
	pathStyle, err := strconv.ParseBool(env("S3_FORCE_PATH_STYLE", "true"))
	if err != nil {
		return Storage{}, fmt.Errorf("S3_FORCE_PATH_STYLE harus true/false")
	}
	allowMinioHTTP, err := strconv.ParseBool(env("S3_ALLOW_MINIO_HTTP", "false"))
	if err != nil {
		return Storage{}, fmt.Errorf("S3_ALLOW_MINIO_HTTP harus true/false")
	}
	cfg := Storage{
		Driver: env("STORAGE_DRIVER", "local"), Directory: env("STORAGE_DIR", "./storage"),
		Endpoint: strings.TrimRight(strings.TrimSpace(os.Getenv("S3_ENDPOINT")), "/"),
		Region:   env("S3_REGION", "auto"), Bucket: strings.TrimSpace(os.Getenv("S3_BUCKET")),
		Prefix:          strings.Trim(strings.TrimSpace(os.Getenv("S3_PREFIX")), "/"),
		AccessKeyID:     strings.TrimSpace(os.Getenv("S3_ACCESS_KEY_ID")),
		SecretAccessKey: strings.TrimSpace(os.Getenv("S3_SECRET_ACCESS_KEY")),
		ForcePathStyle:  pathStyle,
		AllowMinioHTTP:  allowMinioHTTP,
	}
	return cfg, cfg.Validate(env("APP_ENV", "development") == "production")
}

func (s Storage) Validate(production bool) error {
	if s.Driver == "local" {
		return nil
	}
	if s.Driver != "s3" {
		return fmt.Errorf("STORAGE_DRIVER harus local/s3")
	}
	u, err := url.Parse(s.Endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("S3_ENDPOINT harus URL HTTP(S) tanpa credentials/path/query")
	}
	// Explicit exception for this Compose's internal MinIO service only. Never
	// permit plaintext credentials/photo traffic to arbitrary external endpoints.
	internalMinio := s.AllowMinioHTTP && u.Scheme == "http" && u.Host == "minio:9000"
	if production && u.Scheme != "https" && !internalMinio {
		return fmt.Errorf("S3_ENDPOINT wajib HTTPS di production")
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`).MatchString(s.Bucket) || strings.Contains(s.Bucket, "..") {
		return fmt.Errorf("S3_BUCKET wajib nama bucket valid (3–63 karakter)")
	}
	if s.Region == "" || s.AccessKeyID == "" || s.SecretAccessKey == "" {
		return fmt.Errorf("S3_REGION, S3_ACCESS_KEY_ID, S3_SECRET_ACCESS_KEY wajib diisi untuk S3")
	}
	if s.Prefix != "" && (!fs.ValidPath(s.Prefix) || s.Prefix == "." || strings.ContainsAny(s.Prefix, "\\:\x00\r\n")) {
		return fmt.Errorf("S3_PREFIX tidak valid")
	}
	return nil
}
