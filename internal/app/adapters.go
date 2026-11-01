package app

import (
	"context"
	"errors"

	"github.com/haidarrasyad/absensi-api/internal/domain/attendances"
	workreports "github.com/haidarrasyad/absensi-api/internal/domain/work_reports"
)

// attendanceLookup menghubungkan domain hasil kerja dengan repository absen
// tanpa membuat kedua domain saling import.
type attendanceLookup struct {
	repo attendances.Repository
}

func (l attendanceLookup) TodayAttendance(ctx context.Context, employeeID, workDate string) (workreports.AttendanceRef, error) {
	attendance, err := l.repo.FindByEmployeeDate(ctx, employeeID, workDate)
	if errors.Is(err, attendances.ErrNotFound) {
		return workreports.AttendanceRef{}, workreports.ErrNotCheckedIn
	}
	if err != nil {
		return workreports.AttendanceRef{}, err
	}
	return workreports.AttendanceRef{AttendanceID: attendance.AttendanceID, CheckedOut: attendance.CheckOutAt != nil}, nil
}
