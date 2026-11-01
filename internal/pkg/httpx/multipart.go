package httpx

import (
	"errors"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
)

// ParseMultipart membaca form multipart dengan batas ukuran total.
func ParseMultipart(w http.ResponseWriter, r *http.Request, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		return apperror.BadRequest("invalid_form", "Data terlalu besar atau formatnya tidak valid.")
	}
	return nil
}

// OptionalFile mengembalikan file pada field tertentu, atau nil jika tidak dikirim.
func OptionalFile(r *http.Request, field string) (multipart.File, error) {
	file, _, err := r.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.BadRequest("invalid_file", "File foto tidak valid.")
	}
	return file, nil
}

// OptionalFloat membaca angka desimal dari form. Kosong berarti nil.
func OptionalFloat(r *http.Request, field string) (*float64, error) {
	raw := strings.TrimSpace(r.FormValue(field))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, apperror.Validation(map[string]string{field: "Nilai " + field + " Tidak Valid"})
	}
	return &value, nil
}
