package employees

import (
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
	"net/http"
)

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.AdminDetail(r.Context(), r.PathValue("employee_id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, 200, e, "Employee Retrieved Successfully")
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) { h.save(w, r, "") }
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, r.PathValue("employee_id"))
}
func (h *Handler) save(w http.ResponseWriter, r *http.Request, id string) {
	var req WriteRequest
	if err := httpx.DecodeJSONLimit(w, r, &req, 4096); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	e, err := h.svc.Save(r.Context(), id, req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	code, message := 200, "Employee Updated Successfully"
	if id == "" {
		code, message = 201, "Employee Created Successfully"
	}
	httpx.OK(w, code, e, message)
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), r.PathValue("employee_id")); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, 200, nil, "Employee Deleted Successfully")
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	var req StatusRequest
	if err := httpx.DecodeJSONLimit(w, r, &req, 4096); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	result, err := h.svc.SetStatus(r.Context(), r.PathValue("employee_id"), *req.IsActive)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, 200, result, "Employee Status Updated Successfully")
}
