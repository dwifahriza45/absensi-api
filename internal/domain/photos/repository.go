package photos

import (
	"context"
	"errors"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository menyimpan metadata foto di database.
type Repository interface {
	Create(ctx context.Context, photo Photo) (Photo, error)
	FindByID(ctx context.Context, id string) (Photo, error)
}

type postgresRepository struct {
	db *pgxpool.Pool
}

// NewRepository membuat repository foto berbasis PostgreSQL.
func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) Create(ctx context.Context, photo Photo) (Photo, error) {
	err := database.Executor(ctx, r.db).QueryRow(ctx, `
		INSERT INTO photos (id, kind, storage_key, content_type, size_bytes, created_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING photo_id`,
		photo.ID, photo.Kind, photo.StorageKey, photo.ContentType, photo.SizeBytes, photo.CreatedAt,
	).Scan(&photo.PhotoID)
	return photo, err
}

func (r *postgresRepository) FindByID(ctx context.Context, id string) (Photo, error) {
	var photo Photo
	err := database.Executor(ctx, r.db).QueryRow(ctx, `
		SELECT id::text, photo_id, kind, storage_key, content_type, size_bytes, created_at
		FROM photos
		WHERE photo_id = $1`, id,
	).Scan(&photo.ID, &photo.PhotoID, &photo.Kind, &photo.StorageKey, &photo.ContentType, &photo.SizeBytes, &photo.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return Photo{}, ErrNotFound
	}
	return photo, err
}
