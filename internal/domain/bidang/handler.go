package bidang

import (
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
	"net/http"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, 200, items, "Bidang Retrieved Successfully")
}
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	item, err := h.svc.Get(r.Context(), r.PathValue("bidang_id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, 200, item, "Bidang Retrieved Successfully")
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) { h.save(w, r, "") }
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, r.PathValue("bidang_id"))
}
func (h *Handler) save(w http.ResponseWriter, r *http.Request, id string) {
	var req WriteRequest
	if err := httpx.DecodeJSONLimit(w, r, &req, 4096); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	item, err := h.svc.Save(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	code, message := 200, "Bidang Updated Successfully"
	if id == "" {
		code, message = 201, "Bidang Created Successfully"
	}
	httpx.OK(w, code, item, message)
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("bidang_id")); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, 200, nil, "Bidang Deleted Successfully")
}
