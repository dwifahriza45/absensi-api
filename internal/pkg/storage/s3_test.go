package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/pkg/config"
)

// Opt-in only: verifies the configured local MinIO without modifying employee data.
func TestS3LiveConfiguredStorage(t *testing.T) {
	if os.Getenv("TEST_S3_LIVE") != "1" {
		t.Skip("set TEST_S3_LIVE=1 for configured MinIO")
	}
	cfg, err := config.LoadStorage()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Driver != "s3" {
		t.Fatal("live test requires s3")
	}
	store, err := NewS3(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	key := "verification/" + rand.Text() + ".jpg"
	t.Cleanup(func() {
		if err := store.Remove(context.Background(), key); err != nil {
			t.Error("smoke object cleanup failed", err)
		}
	})
	if err = store.Save(ctx, key, []byte("temporary storage verification")); err != nil {
		t.Fatal(err)
	}
	if err = store.Save(ctx, key, []byte("overwrite")); !errors.Is(err, fs.ErrExist) {
		t.Fatal("conditional write failed", err)
	}
	f, err := store.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(data) != "temporary storage verification" {
		t.Fatal("read verification failed")
	}
	url := cfg.Endpoint + "/" + cfg.Bucket + "/"
	if cfg.Prefix != "" {
		url += cfg.Prefix + "/"
	}
	resp, err := http.Get(url + key)
	if err != nil {
		t.Fatal("anonymous check failed")
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bucket must deny anonymous access: %d", resp.StatusCode)
	}
	if cfg.Prefix != "" {
		outsideCfg := cfg
		outsideCfg.Prefix = "forbidden-verification"
		outside, err := NewS3(outsideCfg)
		if err != nil {
			t.Fatal(err)
		}
		defer outside.Close()
		if err = outside.Save(ctx, key, []byte("must fail")); err == nil {
			_ = outside.Remove(ctx, key)
			t.Fatal("application credentials can write outside configured prefix")
		}
	}
	if err = store.Remove(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Open(ctx, key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("deleted object should be missing", err)
	}
}

func TestS3PrivateStorageLifecycle(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(r.URL.Path, "/private-bucket/attendance/") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("request not signed")
		}
		if r.Header.Get("x-amz-acl") != "" {
			t.Error("ACL must not be sent to R2")
		}
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("If-None-Match") != "*" {
				t.Error("conditional create required")
			}
			if r.Header.Get("Cache-Control") != "private, no-store" {
				t.Error("private cache control required")
			}
			if _, exists := objects[r.URL.Path]; exists {
				w.WriteHeader(http.StatusPreconditionFailed)
				io.WriteString(w, `<Error><Code>PreconditionFailed</Code></Error>`)
				return
			}
			objects[r.URL.Path], _ = io.ReadAll(r.Body)
		case http.MethodGet:
			data, ok := objects[r.URL.Path]
			if !ok {
				w.WriteHeader(404)
				io.WriteString(w, `<Error><Code>NoSuchKey</Code></Error>`)
				return
			}
			w.Write(data)
		case http.MethodDelete:
			delete(objects, r.URL.Path)
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()
	s, err := NewS3(config.Storage{Driver: "s3", Endpoint: server.URL, Region: "auto", Bucket: "private-bucket",
		Prefix: "attendance", AccessKeyID: "test-id", SecretAccessKey: "test-secret", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, key := range []string{"../secret", "/secret", "x/../a", "C:/secret", "", "."} {
		if err = s.Save(ctx, key, []byte("bad")); err == nil {
			t.Errorf("invalid key accepted %q", key)
		}
		if f, err := s.Open(ctx, key); err == nil {
			f.Close()
			t.Errorf("invalid read accepted %q", key)
		}
		if err = s.Remove(ctx, key); err == nil {
			t.Errorf("invalid delete accepted %q", key)
		}
	}
	key := "selfie_in/2026/11/01/example.jpg"
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.Save(ctx, key, []byte("pixels")) }()
	}
	wg.Wait()
	close(results)
	success, exists := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, fs.ErrExist) {
			exists++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || exists != 1 {
		t.Fatalf("atomic create: success=%d exists=%d", success, exists)
	}
	f, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(data) != "pixels" {
		t.Fatalf("read: %q %v", data, err)
	}
	if err = s.Remove(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err = s.Remove(ctx, key); err != nil {
		t.Fatal("delete must be idempotent", err)
	}
	if _, err = s.Open(ctx, key); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing object: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = s.Save(cancelled, key, []byte("bad")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestS3AccessDeniedIsNotMissingPhoto(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>Denied</Message></Error>`)
	}))
	defer server.Close()
	s, err := NewS3(config.Storage{Driver: "s3", Endpoint: server.URL, Region: "auto", Bucket: "private-bucket",
		AccessKeyID: "test-id", SecretAccessKey: "test-secret", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.Open(context.Background(), "photo.jpg"); !errors.Is(err, ErrUnavailable) || errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("access denied must be unavailable, not missing: %v", err)
	}
}
