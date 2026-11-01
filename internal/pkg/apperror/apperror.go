// Package apperror berisi error yang aman ditampilkan ke pengguna beserta status HTTP-nya.
package apperror

import "net/http"

// Error adalah error bisnis. Code menandai jenis internal; respons HTTP memakai
// Status sebagai code numerik dan Valid untuk pesan per field frontend.
type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Valid   map[string]string `json:"-"`
}

func (e *Error) Error() string { return e.Message }

// New membuat error dengan status HTTP tertentu.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func BadRequest(code, message string) *Error   { return New(http.StatusBadRequest, code, message) }
func Unauthorized(code, message string) *Error { return New(http.StatusUnauthorized, code, message) }
func Forbidden(code, message string) *Error    { return New(http.StatusForbidden, code, message) }
func NotFound(code, message string) *Error     { return New(http.StatusNotFound, code, message) }
func Conflict(code, message string) *Error     { return New(http.StatusConflict, code, message) }

func Unprocessable(code, message string) *Error {
	return New(http.StatusUnprocessableEntity, code, message)
}

func TooManyRequests(code, message string) *Error {
	return New(http.StatusTooManyRequests, code, message)
}

// Internal dipakai untuk error tak terduga. Detail aslinya hanya ditulis ke log.
func Internal() *Error {
	return New(http.StatusInternalServerError, "internal_error", "Terjadi kesalahan di server. Coba lagi sebentar lagi.")
}

// Validation contains field errors keyed by the request's JSON/form names.
func Validation(fields map[string]string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "validation_failed", Message: "Validation Failed", Valid: fields}
}
