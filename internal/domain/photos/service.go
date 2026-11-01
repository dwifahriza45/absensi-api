package photos

import (
	"context"
	"errors"
	"fmt"
	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/pkg/storage"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

// MaxBytes adalah ukuran maksimal satu foto.
const MaxBytes = 5 << 20

// Service menyimpan dan membuka foto.
type Service struct {
	repo  Repository
	store storage.Storage
	clock shared.Clock
	slots chan struct{}
}

// NewService membuat Service foto.
func NewService(repo Repository, store storage.Storage, clock shared.Clock, workerCounts ...int) *Service {
	workers := 2
	if len(workerCounts) > 0 {
		workers = workerCounts[0]
	}
	if workers < 1 || workers > 8 {
		panic("photo workers must be between 1 and 8")
	}
	return &Service{repo: repo, store: store, clock: clock, slots: make(chan struct{}, workers)}
}

// Save memeriksa isi file (harus JPG/PNG), menyimpannya, lalu mencatat metadatanya.
func (s *Service) Save(ctx context.Context, kind Kind, r io.Reader) (Photo, error) {
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return Photo{}, ctx.Err()
	case <-timer.C:
		return Photo{}, apperror.TooManyRequests("upload_busy", "Upload sedang sibuk. Coba lagi sebentar.")
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return Photo{}, fmt.Errorf("baca foto: %w", err)
	}
	if len(data) > MaxBytes {
		return Photo{}, ErrTooLarge
	}

	data, contentType, extension, err := normalizeImage(data)
	if err != nil {
		return Photo{}, err
	}

	now := s.clock.Now()
	photo := Photo{
		ID:          shared.NewID(),
		Kind:        kind,
		ContentType: contentType,
		SizeBytes:   int64(len(data)),
		CreatedAt:   now,
	}
	photo.StorageKey = fmt.Sprintf("%s/%s/%s.%s", kind, now.Format("2006/01/02"), photo.ID, extension)

	if err := s.store.Save(ctx, photo.StorageKey, data); err != nil {
		return Photo{}, fmt.Errorf("simpan file foto: %w", err)
	}
	storageKey := photo.StorageKey
	registered := database.OnRollback(ctx, func(cleanupCtx context.Context) error { return s.store.Remove(cleanupCtx, storageKey) })
	created, err := s.repo.Create(ctx, photo)
	if err != nil {
		if !registered {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if cleanupErr := s.store.Remove(cleanupCtx, storageKey); cleanupErr != nil {
				slog.Error("photo cleanup failed", "error", cleanupErr)
			}
			cancel()
		}
		return Photo{}, fmt.Errorf("simpan data foto: %w", err)
	}
	return created, nil
}

// Open membuka file foto. Pemanggil wajib menutup reader.
func (s *Service) Open(ctx context.Context, id string) (Photo, io.ReadCloser, error) {
	if !shared.IsPublicID(id, "PHO") {
		return Photo{}, nil, ErrNotFound
	}
	photo, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return Photo{}, nil, err
	}
	file, err := s.store.Open(ctx, photo.StorageKey)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Photo{}, nil, ErrNotFound
		}
		return Photo{}, nil, apperror.New(http.StatusServiceUnavailable, "storage_unavailable", "Penyimpanan foto sedang tidak tersedia. Silakan coba lagi.")
	}
	return photo, file, nil
}
