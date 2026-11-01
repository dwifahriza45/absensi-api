package bidang

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
		row.BidangID = strings.TrimSpace(row.BidangID)
		item := RowResult{Row: line, Name: row.Name, BidangID: row.BidangID}
		switch {
		case row.Problem != "":
			item.Status, item.Message = "error", row.Problem
		case row.BidangID != "" && !shared.IsPublicID(row.BidangID, "BID"):
			item.Status, item.Message = "error", "Format bidang_id tidak valid."
		default:
			if e := validation.Struct(&WriteRequest{Name: row.Name}); e != nil {
				var app *apperror.Error
				item.Status, item.Message = "error", "Nama bidang tidak valid."
				if errors.As(e, &app) && app.Valid["name"] != "" {
					item.Message = app.Valid["name"]
				}
			} else {
				key := row.BidangID + "\x00" + row.Name
				if seen[key] {
					item.Status, item.Message = "skip", "Baris duplikat dalam file."
				} else {
					// Savepoint lets an expected per-row constraint failure leave other rows usable.
					nested, e := tx.Begin(ctx)
					if e != nil {
						return ImportResult{}, e
					}
					var changed bool
					if row.BidangID == "" {
						e = nested.QueryRow(ctx, `INSERT INTO bidang(name) VALUES($1) ON CONFLICT(name) DO NOTHING RETURNING bidang_id`, row.Name).Scan(&item.BidangID)
						if errors.Is(e, pgx.ErrNoRows) {
							e = nested.QueryRow(ctx, `SELECT bidang_id FROM bidang WHERE name=$1`, row.Name).Scan(&item.BidangID)
							item.Status, item.Message = "skip", "Nama bidang sudah terdaftar."
						} else {
							changed = e == nil
						}
					} else {
						var name string
						e = nested.QueryRow(ctx, `SELECT name FROM bidang WHERE bidang_id=$1 FOR UPDATE`, row.BidangID).Scan(&name)
						if errors.Is(e, pgx.ErrNoRows) {
							item.Status, item.Message = "error", "Bidang tidak ditemukan."
							e = nil
						} else if e == nil && name == row.Name {
							item.Status, item.Message = "skip", "Data tidak berubah."
						} else if e == nil {
							_, e = nested.Exec(ctx, `UPDATE bidang SET name=$2,updated_at=now() WHERE bidang_id=$1`, row.BidangID, row.Name)
							changed = e == nil
						}
					}
					if e != nil {
						database.Rollback(nested)
						var pe *pgconn.PgError
						if errors.As(e, &pe) && pe.Code == "23505" {
							item.Status, item.Message = "error", "Nama bidang sudah digunakan bidang lain."
						} else {
							return ImportResult{}, e
						}
					} else {
						if e = nested.Commit(ctx); e != nil {
							return ImportResult{}, e
						}
						seen[key] = true
						if changed {
							item.Status, item.Message = "success", "Bidang berhasil disimpan."
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
