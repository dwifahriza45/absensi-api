package photos

import "github.com/haidarrasyad/absensi-api/internal/pkg/apperror"

var (
	ErrNotFound     = apperror.NotFound("photo_not_found", "Foto tidak ditemukan.")
	ErrInvalidImage = apperror.BadRequest("invalid_image", "File harus berupa foto JPG atau PNG.")
	ErrTooLarge     = apperror.BadRequest("image_too_large", "Ukuran foto maksimal 5 MB.")
)
