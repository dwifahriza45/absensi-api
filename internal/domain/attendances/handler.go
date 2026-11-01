package attendances

import (
	"io"
	"net/http"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/photos"
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
)

const formLimit = photos.MaxBytes + (1 << 20) // foto + field lain

// Handler menerima request HTTP absen.
type Handler struct {
	svc   *Service
	recap *Recap
}

// NewHandler membuat Handler absen.
func NewHandler(svc *Service, recap ...*Recap) *Handler {
	h := &Handler{svc: svc}
	if len(recap) > 0 {
		h.recap = recap[0]
	}
	return h
}
func (h *Handler) Recap(w http.ResponseWriter, r *http.Request) {
	if h.recap == nil {
		http.NotFound(w, r)
		return
	}
	h.recap.ServeHTTP(w, r)
}

func (h *Handler) Today(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.Today(r.Context(), middleware.SubjectFrom(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, http.StatusOK, resp)
}

// checkIn menerima multipart: selfie (file, opsional), lat, lng, accuracy (opsional).
func (h *Handler) CheckIn(w http.ResponseWriter, r *http.Request) {
	selfie, location, err := readPhotoAndLocation(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if selfie != nil {
		defer selfie.Close()
	}

	input := CheckInInput{Location: location}
	if selfie != nil {
		input.Selfie = selfie
	}
	resp, err := h.svc.CheckIn(r.Context(), middleware.SubjectFrom(r.Context()), input)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, http.StatusCreated, resp)
}

// checkOut menerima multipart: selfie, lat, lng, accuracy, reason.
func (h *Handler) CheckOut(w http.ResponseWriter, r *http.Request) {
	selfie, location, err := readPhotoAndLocation(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if selfie != nil {
		defer selfie.Close()
	}

	input := CheckOutInput{Location: location, Reason: r.FormValue("reason")}
	if selfie != nil {
		input.Selfie = selfie
	}
	resp, err := h.svc.CheckOut(r.Context(), middleware.SubjectFrom(r.Context()), input)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, http.StatusOK, resp)
}

func (h *Handler) EarlyLeave(w http.ResponseWriter, r *http.Request) {
	var req EarlyLeaveRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	resp, err := h.svc.RequestEarlyLeave(r.Context(), middleware.SubjectFrom(r.Context()), req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, http.StatusOK, resp)
}

func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := h.svc.AdminList(r.Context(), AdminFilter{
		From:  q.Get("from"),
		To:    q.Get("to"),
		Mitra: q.Get("mitra"),

		Bidang: q.Get("bidang"),
		Query:  q.Get("q"),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, http.StatusOK, items)
}

func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.Version(r.Context(), r.URL.Query().Get("date"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, http.StatusOK, resp)
}

// readPhotoAndLocation membaca field "selfie", "lat", "lng", "accuracy" dari form multipart.
// Lokasi hanya dipakai bila ketiga angka terisi.
func readPhotoAndLocation(w http.ResponseWriter, r *http.Request) (io.ReadCloser, *Location, error) {
	if err := httpx.ParseMultipart(w, r, formLimit); err != nil {
		return nil, nil, err
	}

	file, err := httpx.OptionalFile(r, "selfie")
	if err != nil {
		return nil, nil, err
	}

	lat, err := httpx.OptionalFloat(r, "lat")
	if err != nil {
		return nil, nil, err
	}
	lng, err := httpx.OptionalFloat(r, "lng")
	if err != nil {
		return nil, nil, err
	}
	acc, err := httpx.OptionalFloat(r, "accuracy")
	if err != nil {
		return nil, nil, err
	}

	var location *Location
	if lat != nil && lng != nil && acc != nil {
		location = &Location{Lat: *lat, Lng: *lng, AccuracyM: *acc}
	}

	if file == nil {
		return nil, location, nil
	}
	return file, location, nil
}
