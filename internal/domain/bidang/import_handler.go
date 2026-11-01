package bidang

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
)

func (h *Handler) Template(w http.ResponseWriter, r *http.Request) { h.download(w, r, true) }
func (h *Handler) Export(w http.ResponseWriter, r *http.Request)   { h.download(w, r, false) }
func (h *Handler) download(w http.ResponseWriter, r *http.Request, template bool) {
	items := []Response{}
	if !template {
		var err error
		items, err = h.svc.List(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	data, err := writeExcel(items)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	filename := "bidang.xlsx"
	if template {
		filename = "bidang-template.xlsx"
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	var req ImportRequest
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		httpx.Fail(w, r, apperror.BadRequest("invalid_content_type", "Gunakan multipart/form-data dengan field file atau application/json."))
		return
	}
	if kind == "application/json" {
		if err := httpx.DecodeJSONLimit(w, r, &req, 1<<20); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	} else if kind == "multipart/form-data" {
		r.Body = http.MaxBytesReader(w, r.Body, maxExcelFile+(64<<10))
		reader, err := r.MultipartReader()
		if err != nil {
			httpx.Fail(w, r, apperror.BadRequest("invalid_excel", errExcel.Error()))
			return
		}
		found := false
		for {
			part, e := reader.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				httpx.Fail(w, r, apperror.BadRequest("invalid_excel", errExcel.Error()))
				return
			}
			if found || part.FormName() != "file" || !strings.HasSuffix(strings.ToLower(part.FileName()), ".xlsx") {
				part.Close()
				httpx.Fail(w, r, apperror.BadRequest("invalid_excel", "Kirim tepat satu file .xlsx pada field file."))
				return
			}
			req.Rows, e = readExcel(part)
			part.Close()
			if e != nil {
				var tooLarge *http.MaxBytesError
				if errors.As(e, &tooLarge) {
					httpx.Fail(w, r, apperror.New(413, "file_too_large", "File Excel maksimal 8 MiB."))
				} else {
					httpx.Fail(w, r, apperror.BadRequest("invalid_excel", e.Error()))
				}
				return
			}
			found = true
		}
		if !found {
			httpx.Fail(w, r, apperror.Validation(map[string]string{"file": "File Excel Wajib Diisi"}))
			return
		}
	} else {
		httpx.Fail(w, r, apperror.New(415, "unsupported_media_type", "Gunakan file Excel .xlsx atau JSON rows."))
		return
	}
	result, err := h.svc.Import(r.Context(), req)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, 200, result, "Bidang Import Completed")
}
