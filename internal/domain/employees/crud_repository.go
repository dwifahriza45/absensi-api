package employees

import (
	"context"
	"errors"
	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func employeeWriteError(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		if pe.Code == "23505" && pe.ConstraintName == "employees_nik_key" {
			return apperror.Conflict("employee_nik_exists", "NIK sudah terdaftar pada karyawan lain.")
		}
		if pe.Code == "23505" {
			return apperror.Conflict("employee_phone_exists", "Nomor HP sudah terdaftar pada karyawan lain.")
		}
		if pe.Code == "23503" && pe.ConstraintName == "employees_bidang_id_fkey" {
			return apperror.Validation(map[string]string{"bidang_id": "Bidang Tidak Ditemukan"})
		}
		if pe.Code == "23503" {
			return apperror.Validation(map[string]string{"mitra_id": "Mitra Tidak Ditemukan"})
		}
	}
	return database.ConcurrencyError(err)
}
func (r *postgresRepository) Save(ctx context.Context, id string, e Employee, active *bool) (result Employee, err error) {
	// Same lock as import: avoid edits interleaving with snapshot deactivation.
	err = database.NewCoordinator(r.db).Within(ctx, "employees:import", func(ctx context.Context) error {
		q := database.Executor(ctx, r.db)
		var row pgx.Row
		if id == "" {
			enabled := true
			if active != nil {
				enabled = *active
			}
			row = q.QueryRow(ctx, `INSERT INTO employees(name,mitra_id,bidang_id,phone_hash,is_active,phone_encrypted,nik) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7) RETURNING employee_id`, e.Name, e.MitraID, nullableMitraID(e.BidangID), e.PhoneHash, enabled, e.PhoneEncrypted, e.NIK)
		} else {
			row = q.QueryRow(ctx, `UPDATE employees SET nik=$8,name=$2,mitra_id=$3,bidang_id=$4,phone_hash=CASE WHEN $5='' THEN phone_hash ELSE $5 END,phone_encrypted=CASE WHEN $5='' THEN phone_encrypted ELSE $7 END,is_active=COALESCE($6,is_active),updated_at=now() WHERE employee_id=$1 RETURNING employee_id`, id, e.Name, e.MitraID, nullableMitraID(e.BidangID), e.PhoneHash, active, e.PhoneEncrypted, e.NIK)
		}
		var savedID string
		if err := row.Scan(&savedID); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		// Revoke existing logins when credentials change or account is disabled.
		if id != "" {
			if _, err := q.Exec(ctx, `DELETE FROM sessions WHERE employee_id=$1`, savedID); err != nil {
				return err
			}
		}
		var err error
		result, err = scanEmployee(q.QueryRow(ctx, `SELECT `+selectColumns+` FROM `+employeeSource+` WHERE e.employee_id=$1`, savedID))
		return err
	})
	return result, employeeWriteError(err)
}

// Delete shares the attendance mutation lock and locks the employee row. FK
// constraints also protect against inserts from callers outside the coordinator.
func (r *postgresRepository) Delete(ctx context.Context, id string) error {
	err := database.NewCoordinator(r.db).Within(ctx, "employees:import", func(ctx context.Context) error {
		q := database.Executor(ctx, r.db)
		if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "attendance:"+id); err != nil {
			return err
		}
		var stored string
		if err := q.QueryRow(ctx, `SELECT employee_id FROM employees WHERE employee_id=$1 FOR UPDATE`, id).Scan(&stored); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		var history bool
		if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM attendances WHERE employee_id=$1)`, id).Scan(&history); err != nil {
			return err
		}
		if history {
			return ErrHasAttendance
		}
		// sessions.employee_id ON DELETE CASCADE revokes every login atomically.
		_, err := q.Exec(ctx, `DELETE FROM employees WHERE employee_id=$1`, id)
		return err
	})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23503" {
		return ErrHasAttendance
	}
	return database.ConcurrencyError(err)
}

func (r *postgresRepository) SetStatus(ctx context.Context, id string, active bool) (result Employee, err error) {
	err = database.NewCoordinator(r.db).Within(ctx, "employees:import", func(ctx context.Context) error {
		q := database.Executor(ctx, r.db)
		tag, err := q.Exec(ctx, `UPDATE employees SET is_active=$2,updated_at=now() WHERE employee_id=$1`, id, active)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if !active {
			if _, err = q.Exec(ctx, `DELETE FROM sessions WHERE employee_id=$1`, id); err != nil {
				return err
			}
		}
		result, err = scanEmployee(q.QueryRow(ctx, `SELECT `+selectColumns+` FROM `+employeeSource+` WHERE e.employee_id=$1`, id))
		return err
	})
	return result, database.ConcurrencyError(err)
}
