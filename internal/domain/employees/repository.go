package employees

import (
	"context"
	"errors"
	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository mengakses tabel employees.
type Repository interface {
	SetStatus(context.Context, string, bool) (Employee, error)
	Save(context.Context, string, Employee, *bool) (Employee, error)
	Delete(context.Context, string) error
	Search(ctx context.Context, query string, limit int) ([]Employee, error)
	FindByName(ctx context.Context, name string) (Employee, error)
	FindByNIK(ctx context.Context, nik string) (Employee, error)
	FindByID(ctx context.Context, id string) (Employee, error)
	List(ctx context.Context, filter ListFilter) ([]Employee, error)
	FilterOptions(ctx context.Context) (FilterOptions, error)
	Import(ctx context.Context, rows []Employee, deactivateMissing bool) (ImportResult, error)
}

type postgresRepository struct {
	db *pgxpool.Pool
}

// NewRepository membuat repository pegawai berbasis PostgreSQL.
func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

const selectColumns = `e.id::text, e.employee_id, COALESCE(e.nik,''), e.name, COALESCE(e.mitra_id::text, ''), COALESCE(i.name, ''), COALESCE(e.bidang_id,''), COALESCE(b.name,''), e.phone_hash, e.phone_encrypted, e.is_active, e.created_at, e.updated_at, EXISTS(SELECT 1 FROM attendances a WHERE a.employee_id=e.employee_id)`
const employeeSource = `employees e LEFT JOIN mitra i ON i.mitra_id = e.mitra_id LEFT JOIN bidang b ON b.bidang_id = e.bidang_id`

func scanEmployee(row pgx.Row) (Employee, error) {
	var e Employee
	err := row.Scan(&e.ID, &e.EmployeeID, &e.NIK, &e.Name, &e.MitraID, &e.Mitra, &e.BidangID, &e.Bidang, &e.PhoneHash, &e.PhoneEncrypted, &e.IsActive, &e.CreatedAt, &e.UpdatedAt, &e.HasAttendance)
	return e, err
}

func collectEmployees(rows pgx.Rows) ([]Employee, error) {
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Employee, error) {
		return scanEmployee(row)
	})
}

func (r *postgresRepository) Search(ctx context.Context, query string, limit int) ([]Employee, error) {
	if limit <= 0 || limit > searchLimit {
		limit = searchLimit
	}
	pattern := escapeLike(query)
	rows, err := r.db.Query(ctx, `
		SELECT `+selectColumns+`
		FROM `+employeeSource+`
		WHERE is_active
		  AND (e.name ILIKE $1 || '%' OR e.name ILIKE '% ' || $1 || '%')
		ORDER BY e.name
		LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, err
	}
	return collectEmployees(rows)
}

func (r *postgresRepository) FindByID(ctx context.Context, id string) (Employee, error) {
	row := r.db.QueryRow(ctx, `SELECT `+selectColumns+` FROM `+employeeSource+` WHERE e.employee_id = $1`, id)
	employee, err := scanEmployee(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, ErrNotFound
	}
	return employee, err
}

func (r *postgresRepository) List(ctx context.Context, filter ListFilter) ([]Employee, error) {
	rows, err := r.db.Query(ctx, `SELECT `+selectColumns+` FROM `+employeeSource+`
 WHERE (e.is_active OR $4) AND ($1='' OR i.name=$1) AND ($2='' OR b.name=$2)
 AND ($3='' OR e.name ILIKE '%' || $3 || '%') AND ($5='' OR e.bidang_id=$5)
 AND ($6='' OR e.mitra_id=$6) ORDER BY i.name,e.name`, filter.Mitra, filter.Bidang, escapeLike(filter.Query), filter.IncludeInactive, filter.BidangID, filter.MitraID)
	if err != nil {
		return nil, err
	}
	return collectEmployees(rows)
}

// FilterOptions reads master data, including entries not yet assigned to employees.
func (r *postgresRepository) FilterOptions(ctx context.Context) (FilterOptions, error) {
	var options FilterOptions
	for _, target := range []struct {
		query string
		dest  *[]string
	}{
		{`SELECT name FROM mitra ORDER BY name`, &options.Mitra},
		{`SELECT name FROM bidang ORDER BY name`, &options.Bidang},
	} {
		rows, err := r.db.Query(ctx, target.query)
		if err != nil {
			return FilterOptions{}, err
		}
		values, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return FilterOptions{}, err
		}
		*target.dest = values
	}
	return options, nil
}

// Import matches immutable employee IDs or NIK; legacy employees can be matched by contact.
func (r *postgresRepository) Import(ctx context.Context, rows []Employee, deactivateMissing bool) (result ImportResult, err error) {
	defer func() { err = employeeWriteError(err) }()

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

	// Resolve each institution once inside the employee import transaction.
	mitraIDs := make(map[string]string)
	for _, e := range rows {
		name := strings.TrimSpace(e.Mitra)
		if name == "" {
			continue
		}
		if _, ok := mitraIDs[name]; ok {
			continue
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO mitra(name) VALUES($1)
   ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING mitra_id`, name).Scan(&id); err != nil {
			return result, err
		}
		mitraIDs[name] = id
	}
	bidangIDs := make(map[string]string)
	for _, e := range rows {
		name := strings.TrimSpace(e.Bidang)
		if name == "" {
			continue
		}
		if _, ok := bidangIDs[name]; ok {
			continue
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO bidang(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING bidang_id`, name).Scan(&id); err != nil {
			return result, err
		}
		bidangIDs[name] = id
	}

	batch := &pgx.Batch{}
	targets := make(map[string]bool)
	for _, e := range rows {
		target := e.EmployeeID
		if target != "" {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM employees WHERE employee_id=$1)`, target).Scan(&exists); err != nil {
				return result, err
			}
			if !exists {
				return result, ErrNotFound
			}
		} else {
			// Match NIK first; a legacy employee without NIK can be filled using its existing contact.
			err := tx.QueryRow(ctx, `SELECT employee_id FROM employees WHERE nik=$1 OR (nik IS NULL AND phone_hash=$2) ORDER BY (nik=$1) DESC NULLS LAST LIMIT 1`, e.NIK, e.PhoneHash).Scan(&target)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return result, err
			}
		}
		if target != "" {
			if targets[target] {
				return result, apperror.BadRequest("duplicate_employee", "File berisi lebih dari satu baris untuk karyawan yang sama.")
			}
			targets[target] = true
			batch.Queue(`UPDATE employees SET name=$2,mitra_id=$3,bidang_id=$4,phone_hash=$5,phone_encrypted=$6,nik=$7,is_active=TRUE,updated_at=now() WHERE employee_id=$1 RETURNING false,employee_id`, target, e.Name, nullableMitraID(mitraIDs[strings.TrimSpace(e.Mitra)]), nullableMitraID(bidangIDs[strings.TrimSpace(e.Bidang)]), e.PhoneHash, e.PhoneEncrypted, e.NIK)
		} else {
			batch.Queue(`INSERT INTO employees(name,mitra_id,bidang_id,phone_hash,phone_encrypted,nik) VALUES($1,$2,$3,$4,$5,$6) RETURNING true,employee_id`, e.Name, nullableMitraID(mitraIDs[strings.TrimSpace(e.Mitra)]), nullableMitraID(bidangIDs[strings.TrimSpace(e.Bidang)]), e.PhoneHash, e.PhoneEncrypted, e.NIK)
		}
	}

	results := tx.SendBatch(ctx, batch)
	savedIDs := make([]string, 0, len(rows))
	for range rows {
		var inserted bool
		var savedID string
		if err := results.QueryRow().Scan(&inserted, &savedID); err != nil {
			results.Close()
			return result, err
		}
		savedIDs = append(savedIDs, savedID)
		if inserted {
			result.Created++
		} else {
			result.Updated++
		}
	}
	if err := results.Close(); err != nil {
		return result, err
	}

	if deactivateMissing {
		tag, err := tx.Exec(ctx, `
			UPDATE employees
			SET is_active = FALSE, updated_at = now()
			WHERE is_active AND NOT (employee_id = ANY($1))`, savedIDs)
		if err != nil {
			return result, err
		}
		result.Deactivated = int(tag.RowsAffected())
	}

	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE employee_id=ANY($1) OR employee_id IN (SELECT employee_id FROM employees WHERE NOT is_active)`, savedIDs); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

// escapeLike mencegah karakter % dan _ dari pengguna dianggap wildcard.
func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// FindByName rejects ambiguous names instead of choosing another employee.
func (r *postgresRepository) FindByName(ctx context.Context, name string) (Employee, error) {
	rows, err := r.db.Query(ctx, `SELECT `+selectColumns+` FROM `+employeeSource+` WHERE e.is_active AND lower(trim(e.name)) = lower($1) LIMIT 2`, name)
	if err != nil {
		return Employee{}, err
	}
	defer rows.Close()
	items, err := collectEmployees(rows)
	if err != nil {
		return Employee{}, err
	}
	if len(items) != 1 {
		return Employee{}, ErrNotFound
	}
	return items[0], nil
}

func nullableMitraID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func (r *postgresRepository) FindByNIK(ctx context.Context, nik string) (Employee, error) {
	e, err := scanEmployee(r.db.QueryRow(ctx, `SELECT `+selectColumns+` FROM `+employeeSource+` WHERE e.nik=$1`, nik))
	if errors.Is(err, pgx.ErrNoRows) {
		return Employee{}, ErrNotFound
	}
	return e, err
}
