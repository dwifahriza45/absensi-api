package auth

import "github.com/haidarrasyad/absensi-api/internal/pkg/apperror"

var (
	ErrInvalidCredentials = apperror.Unauthorized("invalid_credentials",
		"Data login tidak cocok atau akun tidak aktif.")
	ErrInvalidRefreshToken = apperror.Unauthorized("invalid_refresh_token",
		"Cookie refresh tidak ada, tidak valid, atau sesi sudah berakhir. Silakan masuk lagi.")
	ErrTooManyAttempts = apperror.TooManyRequests("too_many_attempts",
		"Terlalu banyak percobaan. Silakan coba lagi dalam 15 menit, atau hubungi PIC Anda.")
)
