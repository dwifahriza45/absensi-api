package photos

import (
	"io"
	"net/http"
	"strconv"

	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
)

// Handler menyajikan file foto untuk admin.
type Handler struct {
	svc *Service
}

// NewHandler membuat Handler foto.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	photo, file, err := h.svc.Open(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer file.Close()

	w.Header().Set("X-Photo-ID", photo.PhotoID)
	w.Header().Set("Content-Type", photo.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(photo.SizeBytes, 10))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, file)
}
