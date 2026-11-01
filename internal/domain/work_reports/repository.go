package workreports

import (
	"context"
	"errors"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository mengakses tabel work_reports.
type Repository interface {
	FindByAttendanceSession(ctx context.Context, attendanceID string, sessionNo int) (WorkReport, error)
	Create(ctx context.Context, wr WorkReport) (WorkReport, bool, error)
	SubmittedSessions(ctx context.Context, attendanceID string) (map[int]time.Time, error)
}

type postgresRepository struct {
	db *pgxpool.Pool
}

// NewRepository membuat repository hasil kerja berbasis PostgreSQL.
func NewRepository(db *pgxpool.Pool) Repository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) FindByAttendanceSession(ctx context.Context, attendanceID string, sessionNo int) (WorkReport, error) {
	wr := WorkReport{AttendanceID: attendanceID, SessionNo: sessionNo}
	err := database.Executor(ctx, r.db).QueryRow(ctx, `
		SELECT id::text, work_report_id, photo_id::text, received_at
		FROM work_reports
		WHERE attendance_id = $1 AND session_no = $2`, attendanceID, sessionNo,
	).Scan(&wr.ID, &wr.WorkReportID, &wr.PhotoID, &wr.ReceivedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return WorkReport{}, ErrNotFound
	}
	return wr, err
}

// Create menyimpan foto sesi. Bila sesi sudah ada (kirim ulang bersamaan), data lama dikembalikan.
func (r *postgresRepository) Create(ctx context.Context, wr WorkReport) (WorkReport, bool, error) {
	err := database.Executor(ctx, r.db).QueryRow(ctx, `
		WITH open_attendance AS (
 SELECT attendance_id FROM attendances WHERE attendance_id=$2 AND check_out_at IS NULL FOR UPDATE
 )
 INSERT INTO work_reports (id, attendance_id, session_no, photo_id)
 SELECT $1,attendance_id,$3,$4 FROM open_attendance
		ON CONFLICT (attendance_id, session_no) DO NOTHING
		RETURNING work_report_id, received_at`,
		wr.ID, wr.AttendanceID, wr.SessionNo, wr.PhotoID,
	).Scan(&wr.WorkReportID, &wr.ReceivedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		existing, findErr := r.FindByAttendanceSession(ctx, wr.AttendanceID, wr.SessionNo)
		if errors.Is(findErr, ErrNotFound) {
			return WorkReport{}, false, ErrAlreadyCheckedOut
		}
		return existing, false, findErr
	}
	if err != nil {
		return WorkReport{}, false, err
	}
	return wr, true, nil
}

func (r *postgresRepository) SubmittedSessions(ctx context.Context, attendanceID string) (map[int]time.Time, error) {
	rows, err := database.Executor(ctx, r.db).Query(ctx, `
		SELECT session_no, received_at
		FROM work_reports
		WHERE attendance_id = $1`, attendanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	submitted := make(map[int]time.Time)
	for rows.Next() {
		var no int
		var receivedAt time.Time
		if err := rows.Scan(&no, &receivedAt); err != nil {
			return nil, err
		}
		submitted[no] = receivedAt
	}
	return submitted, rows.Err()
}
