package workreports

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/photos"
	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
)

const formLimit = photos.MaxBytes + (1 << 20)

// Handler menerima foto hasil kerja dari aplikasi pegawai.
type Handler struct {
	svc *Service
}

// NewHandler membuat Handler hasil kerja.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// submit menerima multipart: session_no (angka) dan photo (file).
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	if err := httpx.ParseMultipart(w, r, formLimit); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	fields := map[string]string{}
	rawSession := strings.TrimSpace(r.FormValue("session_no"))
	sessionNo, err := strconv.Atoi(rawSession)
	if rawSession == "" {
		fields["session_no"] = "Sesi Foto Wajib Diisi"
	} else if err != nil || sessionNo < 1 {
		fields["session_no"] = "Sesi Foto Tidak Valid"
	}
	file, err := httpx.OptionalFile(r, "photo")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if file == nil {
		fields["photo"] = "Foto Wajib Diisi"
	}
	if len(fields) > 0 {
		if file != nil {
			_ = file.Close()
		}
		httpx.Fail(w, r, apperror.Validation(fields))
		return
	}

	defer file.Close()

	resp, err := h.svc.Submit(r.Context(), middleware.SubjectFrom(r.Context()), sessionNo, file)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	status := http.StatusCreated
	if resp.Duplicate {
		status = http.StatusOK
	}
	httpx.OK(w, status, resp)
}
