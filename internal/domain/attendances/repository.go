package attendances

import (
	"context"
	"errors"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository mengakses tabel attendances.
type Repository interface {
	FindByEmployeeDate(ctx context.Context, employeeID, workDate string) (Attendance, error)
	Create(ctx context.Context, a Attendance) (Attendance, error)
	UpdateCheckOut(ctx context.Context, a Attendance) error
	UpdateEarlyLeave(ctx context.Context, a Attendance) error
	ListForAdmin(ctx context.Context, filter AdminFilter) ([]AdminRecord, error)
	LatestChange(ctx context.Context, workDate string) (time.Time, error)
}

type postgresRepository struct {
	db *pgxpool.Pool
}

// NewRepository membuat repository absen berbasis PostgreSQL.
func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

const columns = `
	a.id::text, a.attendance_id, a.employee_id::text, a.work_date::text,
	a.check_in_at, a.check_in_lat, a.check_in_lng, a.check_in_accuracy_m, a.check_in_selfie_id::text, a.check_in_flag,
	a.check_out_at, a.check_out_lat, a.check_out_lng, a.check_out_accuracy_m, a.check_out_selfie_id::text, a.check_out_reason,
	a.early_leave_requested_at, a.early_leave_reason, a.created_at, a.updated_at`

// scanAttendance membaca kolom di atas. extra dipakai untuk kolom tambahan (mis. data pegawai).
func scanAttendance(row pgx.Row, extra ...any) (Attendance, error) {
	var a Attendance
	var inLat, inLng, inAcc, outLat, outLng, outAcc *float64

	dest := []any{
		&a.ID, &a.AttendanceID, &a.EmployeeID, &a.WorkDate,
		&a.CheckInAt, &inLat, &inLng, &inAcc, &a.CheckInSelfieID, &a.CheckInFlag,
		&a.CheckOutAt, &outLat, &outLng, &outAcc, &a.CheckOutSelfieID, &a.CheckOutReason,
		&a.EarlyLeaveRequestedAt, &a.EarlyLeaveReason, &a.CreatedAt, &a.UpdatedAt,
	}
	if err := row.Scan(append(dest, extra...)...); err != nil {
		return Attendance{}, err
	}

	a.CheckInLocation = locationFrom(inLat, inLng, inAcc)
	a.CheckOutLocation = locationFrom(outLat, outLng, outAcc)
	return a, nil
}

func locationFrom(lat, lng, acc *float64) *Location {
	if lat == nil || lng == nil || acc == nil {
		return nil
	}
	return &Location{Lat: *lat, Lng: *lng, AccuracyM: *acc}
}

func locationParts(l *Location) (lat, lng, acc *float64) {
	if l == nil {
		return nil, nil, nil
	}
	return &l.Lat, &l.Lng, &l.AccuracyM
}

func (r *postgresRepository) FindByEmployeeDate(ctx context.Context, employeeID, workDate string) (Attendance, error) {
	row := database.Executor(ctx, r.db).QueryRow(ctx, `
		SELECT `+columns+`
		FROM attendances a
		WHERE a.employee_id = $1 AND a.work_date = $2::date`, employeeID, workDate)

	attendance, err := scanAttendance(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attendance{}, ErrNotFound
	}
	return attendance, err
}

func (r *postgresRepository) Create(ctx context.Context, a Attendance) (Attendance, error) {
	lat, lng, acc := locationParts(a.CheckInLocation)
	err := database.Executor(ctx, r.db).QueryRow(ctx, `
		INSERT INTO attendances (
			id, employee_id, work_date, check_in_at,
			check_in_lat, check_in_lng, check_in_accuracy_m, check_in_selfie_id, check_in_flag
		) VALUES ($1, $2, $3::date, $4, $5, $6, $7, $8, $9)
		RETURNING attendance_id, created_at, updated_at`,
		a.ID, a.EmployeeID, a.WorkDate, a.CheckInAt, lat, lng, acc, a.CheckInSelfieID, a.CheckInFlag,
	).Scan(&a.AttendanceID, &a.CreatedAt, &a.UpdatedAt)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation: sudah absen hari ini
		return Attendance{}, ErrAlreadyCheckedIn
	}
	return a, err
}

func (r *postgresRepository) UpdateCheckOut(ctx context.Context, a Attendance) error {
	lat, lng, acc := locationParts(a.CheckOutLocation)
	tag, err := database.Executor(ctx, r.db).Exec(ctx, `
		UPDATE attendances
		SET check_out_at = $2, check_out_lat = $3, check_out_lng = $4, check_out_accuracy_m = $5,
		    check_out_selfie_id = $6, check_out_reason = $7, updated_at = now()
		WHERE attendance_id = $1 AND check_out_at IS NULL`,
		a.AttendanceID, a.CheckOutAt, lat, lng, acc, a.CheckOutSelfieID, a.CheckOutReason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAlreadyCheckedOut
	}
	return nil
}

func (r *postgresRepository) UpdateEarlyLeave(ctx context.Context, a Attendance) error {
	tag, err := database.Executor(ctx, r.db).Exec(ctx, `
		UPDATE attendances
		SET early_leave_requested_at = $2, early_leave_reason = $3, updated_at = now()
		WHERE attendance_id = $1 AND early_leave_requested_at IS NULL AND check_out_at IS NULL`,
		a.AttendanceID, a.EarlyLeaveRequestedAt, a.EarlyLeaveReason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrEarlyLeaveExists
	}
	return nil
}

func (r *postgresRepository) ListForAdmin(ctx context.Context, f AdminFilter) ([]AdminRecord, error) {
	rows, err := database.Executor(ctx, r.db).Query(ctx, `
		SELECT `+columns+`, e.id::text, e.name, COALESCE(i.mitra_id, ''), COALESCE(i.name, ''), COALESCE(b.name,'')
		FROM attendances a
		JOIN employees e ON e.employee_id = a.employee_id
		LEFT JOIN mitra i ON i.mitra_id = e.mitra_id
 LEFT JOIN bidang b ON b.bidang_id=e.bidang_id
		WHERE a.work_date BETWEEN $1::date AND $2::date
		  AND ($3 = '' OR i.name = $3)
		  AND ($4 = '' OR b.name = $4)
		  AND ($5 = '' OR e.name ILIKE '%' || $5 || '%')
		ORDER BY a.work_date DESC, a.updated_at DESC`,
		f.From, f.To, f.Mitra, f.Bidang, f.Query)
	if err != nil {
		return nil, err
	}

	records, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AdminRecord, error) {
		var rec AdminRecord
		attendance, err := scanAttendance(row, &rec.EmployeeUUID, &rec.EmployeeName, &rec.MitraCode, &rec.Mitra, &rec.Bidang)
		rec.Attendance = attendance
		return rec, err
	})
	if err != nil || len(records) == 0 {
		return records, err
	}

	return records, r.attachSessions(ctx, records)
}

// attachSessions mengambil foto hasil kerja untuk semua absen sekaligus (satu query).
func (r *postgresRepository) attachSessions(ctx context.Context, records []AdminRecord) error {
	ids := make([]string, len(records))
	index := make(map[string]int, len(records))
	for i, rec := range records {
		ids[i] = rec.AttendanceID
		index[rec.AttendanceID] = i
	}

	rows, err := database.Executor(ctx, r.db).Query(ctx, `
		SELECT attendance_id::text, session_no, photo_id::text, received_at
		FROM work_reports
		WHERE attendance_id = ANY($1::text[])
		ORDER BY session_no`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var attendanceID string
		var session SubmittedSession
		if err := rows.Scan(&attendanceID, &session.No, &session.PhotoID, &session.ReceivedAt); err != nil {
			return err
		}
		i := index[attendanceID]
		records[i].Sessions = append(records[i].Sessions, session)
	}
	return rows.Err()
}

func (r *postgresRepository) LatestChange(ctx context.Context, workDate string) (time.Time, error) {
	var latest time.Time
	err := database.Executor(ctx, r.db).QueryRow(ctx, `
		SELECT COALESCE(GREATEST(
			(SELECT MAX(updated_at) FROM attendances WHERE work_date = $1::date),
			(SELECT MAX(w.received_at)
			   FROM work_reports w
			   JOIN attendances a ON a.attendance_id = w.attendance_id
			  WHERE a.work_date = $1::date)
		), 'epoch'::timestamptz)`, workDate).Scan(&latest)
	return latest, err
}
