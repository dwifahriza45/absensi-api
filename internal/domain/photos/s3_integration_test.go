package photos

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/pkg/config"
	"github.com/haidarrasyad/absensi-api/internal/pkg/storage"
)

type memoryPhotoRepo struct {
	photo Photo
	fail  bool
}

func (r *memoryPhotoRepo) Create(_ context.Context, p Photo) (Photo, error) {
	if r.fail {
		return Photo{}, errors.New("simulated DB failure")
	}
	p.PhotoID = "PHO-20261101-000001"
	r.photo = p
	return p, nil
}
func (r *memoryPhotoRepo) FindByID(_ context.Context, _ string) (Photo, error) { return r.photo, nil }

type photoClock struct{}

func (photoClock) Now() time.Time { return time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC) }

func TestS3PhotoSaveReadAndRollbackCompensation(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	denyReads := false
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case "PUT":
			objects[r.URL.Path], _ = io.ReadAll(r.Body)
		case "GET":
			if denyReads {
				w.WriteHeader(403)
				io.WriteString(w, `<Error><Code>AccessDenied</Code></Error>`)
				return
			}
			if b, ok := objects[r.URL.Path]; ok {
				w.Write(b)
			} else {
				w.WriteHeader(404)
				io.WriteString(w, `<Error><Code>NoSuchKey</Code></Error>`)
			}
		case "DELETE":
			delete(objects, r.URL.Path)
			w.WriteHeader(204)
		}
	}))
	defer remote.Close()
	store, err := storage.NewS3(config.Storage{Driver: "s3", Endpoint: remote.URL, Region: "auto", Bucket: "test-bucket", AccessKeyID: "fake", SecretAccessKey: "fake", ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repo := &memoryPhotoRepo{fail: true}
	svc := NewService(repo, store, photoClock{})
	var input bytes.Buffer
	if err = png.Encode(&input, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Save(context.Background(), KindSelfieIn, bytes.NewReader(input.Bytes())); err == nil {
		t.Fatal("simulated database failure not returned")
	}
	mu.Lock()
	remaining := len(objects)
	mu.Unlock()
	if remaining != 0 {
		t.Fatal("rollback left uploaded object behind")
	}
	repo.fail = false
	p, err := svc.Save(context.Background(), KindSelfieIn, bytes.NewReader(input.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/photos/"+p.PhotoID, nil)
	request.SetPathValue("id", p.PhotoID)
	response := httptest.NewRecorder()
	NewHandler(svc).Show(response, request)
	if response.Code != 200 || response.Header().Get("Content-Type") != "image/png" || response.Header().Get("Cache-Control") != "no-store" || int64(response.Body.Len()) != p.SizeBytes {
		t.Fatalf("authenticated API photo stream contract changed: code=%d", response.Code)
	}
	if _, err = png.Decode(bytes.NewReader(response.Body.Bytes())); err != nil {
		t.Fatal("photo corrupted", err)
	}
	mu.Lock()
	denyReads = true
	mu.Unlock()
	response = httptest.NewRecorder()
	NewHandler(svc).Show(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("storage permission failure became %d", response.Code)
	}
}
