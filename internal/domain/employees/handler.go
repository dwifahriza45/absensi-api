package employees

import (
	"net/http"

	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
)

const importBodyLimit = 4 << 20 // 4 MB, cukup untuk 5.000 baris

// Handler menerima request HTTP untuk data pegawai.
type Handler struct {
	svc *Service
}

// NewHandler membuat Handler pegawai.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, http.StatusOK, items)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, err := h.svc.List(r.Context(), ListFilter{
		IncludeInactive: q.Get("includeInactive") == "true",
		Mitra:           q.Get("mitra"),
		MitraID:         q.Get("mitra_id"),

		Bidang:   q.Get("bidang"),
		BidangID: q.Get("bidang_id"),
		Query:    q.Get("q"),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, http.StatusOK, items)
}

func (h *Handler) FilterOptions(w http.ResponseWriter, r *http.Request) {
	options, err := h.svc.FilterOptions(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, http.StatusOK, options)
}

func (h *Handler) ImportRows(w http.ResponseWriter, r *http.Request) {
	var req ImportRequest
	if err := httpx.DecodeJSONLimit(w, r, &req, importBodyLimit); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := h.svc.Import(r.Context(), req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, http.StatusOK, result)
}
