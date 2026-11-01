// Package httpx berisi helper HTTP yang dipakai semua handler.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"

	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/validation"
)

// Response follows coffee-api's JSON envelope. HTTP status equals Code.
type Response struct {
	Code    int               `json:"code"`
	Status  string            `json:"status"`
	Message string            `json:"message,omitempty"`
	Data    any               `json:"data,omitempty"`
	Total   *int              `json:"total,omitempty"`
	Error   bool              `json:"error"`
	Valid   map[string]string `json:"valid,omitempty"`
}

// OK writes a successful response; an optional message describes the action.
func OK(w http.ResponseWriter, status int, data any, messages ...string) {
	message := "Success"
	if len(messages) > 0 {
		message = messages[0]
	}
	write(w, status, Response{Code: status, Status: "OK", Message: message, Data: data, Error: false})
}

// Fail menulis respons error. Error yang bukan apperror dianggap 500 dan dicatat di log.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		slog.ErrorContext(r.Context(), "error tak terduga", "method", r.Method, "path", r.URL.Path, "error", err)
		appErr = apperror.Internal()
	}
	write(w, appErr.Status, Response{Code: appErr.Status, Status: "NOK", Message: appErr.Message, Error: true, Valid: appErr.Valid})
}

func write(w http.ResponseWriter, status int, body Response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

const defaultJSONLimit = 1 << 20 // 1 MB

// DecodeJSON membaca body JSON (maks 1 MB) dan menolak field yang tidak dikenal.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	return DecodeJSONLimit(w, r, dst, defaultJSONLimit)
}

// DecodeJSONLimit sama dengan DecodeJSON dengan batas ukuran khusus.
func DecodeJSONLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return apperror.BadRequest("invalid_json", "Format data tidak valid.")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return apperror.BadRequest("invalid_json", "Body harus berisi satu objek JSON.")
	}
	return validation.Struct(dst)
}

// ClientIP mengambil IP pengirim dari koneksi langsung.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
