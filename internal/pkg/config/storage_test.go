package config

import "testing"

func TestStorageConfiguration(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "s3")
	t.Setenv("APP_ENV", "production")
	t.Setenv("S3_ENDPOINT", "https://account.r2.cloudflarestorage.com")
	t.Setenv("S3_BUCKET", "absensi-production")
	t.Setenv("S3_REGION", "auto")
	t.Setenv("S3_PREFIX", "")
	t.Setenv("S3_ACCESS_KEY_ID", "test-id")
	t.Setenv("S3_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("S3_FORCE_PATH_STYLE", "true")
	if _, err := LoadStorage(); err != nil {
		t.Fatal(err)
	}
	for key, bad := range map[string]string{
		"S3_ENDPOINT": "http://account.r2.cloudflarestorage.com", "S3_BUCKET": "../bucket", "S3_PREFIX": "../private",
		"S3_ACCESS_KEY_ID": "", "S3_SECRET_ACCESS_KEY": "", "S3_FORCE_PATH_STYLE": "typo", "STORAGE_DRIVER": "r2-typo",
		"S3_ALLOW_MINIO_HTTP": "typo",
	} {
		old := env(key, "")
		t.Setenv(key, bad)
		if _, err := LoadStorage(); err == nil {
			t.Errorf("invalid %s accepted", key)
		}
		t.Setenv(key, old)
	}
	for _, bad := range []string{"https://id:secret@host", "https://host?token=secret", "https://host/bucket"} {
		t.Setenv("S3_ENDPOINT", bad)
		if _, err := LoadStorage(); err == nil {
			t.Fatal("endpoint with credentials/query/path accepted")
		}
	}
}

func TestProductionMinioHTTPExceptionIsExplicitAndBounded(t *testing.T) {
	cfg := Storage{Driver: "s3", Endpoint: "http://minio:9000", Region: "us-east-1", Bucket: "private-bucket", AccessKeyID: "test", SecretAccessKey: "test"}
	if err := cfg.Validate(true); err == nil {
		t.Fatal("insecure endpoint allowed without explicit opt-in")
	}
	cfg.AllowMinioHTTP = true
	if err := cfg.Validate(true); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"http://external.example:9000", "http://account.r2.cloudflarestorage.com", "http://minio.evil.test:9000", "http://minio:9001", "http://minio:9000?x=1", "http://user:secret@minio:9000"} {
		cfg.Endpoint = endpoint
		if err := cfg.Validate(true); err == nil {
			t.Fatal("external/invalid HTTP endpoint allowed", endpoint)
		}
	}
}
