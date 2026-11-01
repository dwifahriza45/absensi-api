package mitra

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"github.com/haidarrasyad/absensi-api/internal/pkg/validation"
	"github.com/haidarrasyad/absensi-api/internal/shared"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) Import(ctx context.Context, req ImportRequest) (ImportResult, error) {
	if err := validation.Struct(&req); err != nil {
		return ImportResult{}, err
	}
	result, err := s.repo.Import(ctx, req.Rows)
	return result, convert(err)
}

func (r *Repository) Import(ctx context.Context, rows []ImportRow) (result ImportResult, err error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer database.Rollback(tx)
	if err = database.ConfigureTransaction(ctx, tx); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('employees:import',0))`); err != nil {
		return result, err
	}
	result = ImportResult{Total: len(rows), Rows: make([]RowResult, 0, len(rows))}
	seen := map[string]bool{}
	for i, row := range rows {
		line := row.Row
		if line == 0 {
			line = i + 2
		}
		row.Name = strings.TrimSpace(row.Name)
		row.MitraID = strings.TrimSpace(row.MitraID)
		item := RowResult{Row: line, Name: row.Name, MitraID: row.MitraID}
		switch {
		case row.Problem != "":
			item.Status, item.Message = "error", row.Problem
		case row.MitraID != "" && !shared.IsPublicID(row.MitraID, "MTR"):
			item.Status, item.Message = "error", "Format mitra_id tidak valid."
		default:
			if e := validation.Struct(&WriteRequest{Name: row.Name}); e != nil {
				var app *apperror.Error
				item.Status, item.Message = "error", "Nama mitra tidak valid."
				if errors.As(e, &app) && app.Valid["name"] != "" {
					item.Message = app.Valid["name"]
				}
			} else {
				key := row.MitraID + "\x00" + row.Name
				if seen[key] {
					item.Status, item.Message = "skip", "Baris duplikat dalam file."
				} else {
					// Savepoint lets an expected per-row constraint failure leave other rows usable.
					nested, e := tx.Begin(ctx)
					if e != nil {
						return ImportResult{}, e
					}
					var changed bool
					if row.MitraID == "" {
						e = nested.QueryRow(ctx, `INSERT INTO mitra(name) VALUES($1) ON CONFLICT(name) DO NOTHING RETURNING mitra_id`, row.Name).Scan(&item.MitraID)
						if errors.Is(e, pgx.ErrNoRows) {
							e = nested.QueryRow(ctx, `SELECT mitra_id FROM mitra WHERE name=$1`, row.Name).Scan(&item.MitraID)
							item.Status, item.Message = "skip", "Nama mitra sudah terdaftar."
						} else {
							changed = e == nil
						}
					} else {
						var name string
						e = nested.QueryRow(ctx, `SELECT name FROM mitra WHERE mitra_id=$1 FOR UPDATE`, row.MitraID).Scan(&name)
						if errors.Is(e, pgx.ErrNoRows) {
							item.Status, item.Message = "error", "Mitra tidak ditemukan."
							e = nil
						} else if e == nil && name == row.Name {
							item.Status, item.Message = "skip", "Data tidak berubah."
						} else if e == nil {
							_, e = nested.Exec(ctx, `UPDATE mitra SET name=$2,updated_at=now() WHERE mitra_id=$1`, row.MitraID, row.Name)
							changed = e == nil
						}
					}
					if e != nil {
						database.Rollback(nested)
						var pe *pgconn.PgError
						if errors.As(e, &pe) && pe.Code == "23505" {
							item.Status, item.Message = "error", "Nama mitra sudah digunakan mitra lain."
						} else {
							return ImportResult{}, e
						}
					} else {
						if e = nested.Commit(ctx); e != nil {
							return ImportResult{}, e
						}
						seen[key] = true
						if changed {
							item.Status, item.Message = "success", "Mitra berhasil disimpan."
						}
					}
				}
			}
		}
		switch item.Status {
		case "success":
			result.Success++
		case "skip":
			result.Skip++
		default:
			result.Error++
		}
		result.Rows = append(result.Rows, item)
	}
	if err = tx.Commit(ctx); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}
