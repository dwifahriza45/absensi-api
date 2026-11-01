package middleware

import (
	"net/http"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
)

// UploadAdmission bounds multipart body allocations before parsing. Waiting
// requests keep bodies unread, so a burst cannot allocate 6 MiB per employee.
func UploadAdmission(maxActive int, wait time.Duration) Middleware {
	if maxActive < 1 {
		panic("upload admission requires a positive limit")
	}
	slots := make(chan struct{}, maxActive)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				next.ServeHTTP(w, r)
			case <-timer.C:
				w.Header().Set("Retry-After", "2")
				httpx.Fail(w, r, apperror.New(http.StatusServiceUnavailable, "upload_busy", "Antrean upload penuh. Coba lagi sebentar."))
			case <-r.Context().Done():
				return
			}
		})
	}
}
