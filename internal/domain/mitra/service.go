package mitra

import (
	"context"
	"errors"
	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"github.com/haidarrasyad/absensi-api/internal/pkg/validation"
	"github.com/haidarrasyad/absensi-api/internal/shared"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
)

var ErrNotFound = apperror.NotFound("mitra_not_found", "Mitra tidak ditemukan.")

type Service struct{ repo *Repository }

func NewService(repo *Repository) *Service { return &Service{repo: repo} }
func convert(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		if pe.Code == "23505" {
			return apperror.Conflict("mitra_exists", "Nama mitra sudah terdaftar.")
		}
		if pe.Code == "23503" {
			return apperror.Conflict("mitra_in_use", "Mitra masih dipakai karyawan dan tidak dapat dihapus.")
		}
	}
	return database.ConcurrencyError(err)
}
func (s *Service) List(ctx context.Context, q string) ([]Response, error) {
	items, err := s.repo.List(ctx, strings.TrimSpace(q))
	return items, convert(err)
}
func (s *Service) Get(ctx context.Context, id string) (Response, error) {
	if !shared.IsPublicID(id, "MTR") {
		return Response{}, ErrNotFound
	}
	item, err := s.repo.Get(ctx, id)
	return item, convert(err)
}
func (s *Service) Save(ctx context.Context, id string, req WriteRequest) (Response, error) {
	if id != "" && !shared.IsPublicID(id, "MTR") {
		return Response{}, ErrNotFound
	}
	if err := validation.Struct(&req); err != nil {
		return Response{}, err
	}
	item, err := s.repo.Save(ctx, id, req.Name)
	return item, convert(err)
}
func (s *Service) Delete(ctx context.Context, id string) error {
	if !shared.IsPublicID(id, "MTR") {
		return ErrNotFound
	}
	return convert(s.repo.Delete(ctx, id))
}
