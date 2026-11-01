package workreports

import (
	"context"
	"errors"
	"io"

	"github.com/haidarrasyad/absensi-api/internal/domain/photos"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

// AttendanceRef adalah data absen hari ini yang dibutuhkan untuk kirim foto.
type AttendanceRef struct {
	AttendanceID string
	CheckedOut   bool
}

// AttendanceLookup mencari absen hari ini. Wajib mengembalikan ErrNotCheckedIn bila belum absen.
type AttendanceLookup interface {
	TodayAttendance(ctx context.Context, employeeID, workDate string) (AttendanceRef, error)
}

// PhotoSaver adalah kebutuhan hasil kerja terhadap domain foto.
type PhotoSaver interface {
	Save(ctx context.Context, kind photos.Kind, r io.Reader) (photos.Photo, error)
}

// Service berisi aturan bisnis foto hasil kerja.
type Service struct {
	repo        Repository
	photos      PhotoSaver
	attendances AttendanceLookup
	clock       shared.Clock
	schedule    shared.Schedule
	coordinator shared.MutationCoordinator
}

// NewService membuat Service hasil kerja.
func NewService(repo Repository, photos PhotoSaver, attendances AttendanceLookup, clock shared.Clock, schedule shared.Schedule, coordinator shared.MutationCoordinator) *Service {
	return &Service{repo: repo, photos: photos, attendances: attendances, clock: clock, schedule: schedule, coordinator: coordinator}
}

// Submit menyimpan foto untuk satu sesi. Bisa dikirim tanpa batas jam sesi setelah absen masuk dan sebelum pulang.
// Kirim ulang untuk sesi yang sama mengembalikan data lama, tidak membuat data ganda.
func (s *Service) submit(ctx context.Context, employeeID string, sessionNo int, photo io.Reader) (SubmitResponse, error) {
	if photo == nil {
		return SubmitResponse{}, ErrPhotoRequired
	}
	_, ok := s.schedule.SessionByNo(sessionNo)
	if !ok {
		return SubmitResponse{}, ErrInvalidSession
	}

	now := s.clock.Now()
	attendance, err := s.attendances.TodayAttendance(ctx, employeeID, shared.DateString(now))
	if err != nil {
		return SubmitResponse{}, err
	}

	existing, err := s.repo.FindByAttendanceSession(ctx, attendance.AttendanceID, sessionNo)
	if err == nil {
		return toResponse(existing, true), nil
	}
	if !errors.Is(err, ErrNotFound) {
		return SubmitResponse{}, err
	}

	if attendance.CheckedOut {
		return SubmitResponse{}, ErrAlreadyCheckedOut
	}

	saved, err := s.photos.Save(ctx, photos.KindWorkReport, photo)
	if err != nil {
		return SubmitResponse{}, err
	}

	report, created, err := s.repo.Create(ctx, WorkReport{
		ID:           shared.NewID(),
		AttendanceID: attendance.AttendanceID,
		SessionNo:    sessionNo,
		PhotoID:      saved.PhotoID,
	})
	if err != nil {
		return SubmitResponse{}, err
	}
	return toResponse(report, !created), nil
}

func (s *Service) Submit(ctx context.Context, employeeID string, sessionNo int, photo io.Reader) (SubmitResponse, error) {
	var result SubmitResponse
	err := s.coordinator.Within(ctx, "attendance:"+employeeID, func(ctx context.Context) error {
		var err error
		result, err = s.submit(ctx, employeeID, sessionNo, photo)
		return err
	})
	if err != nil {
		return SubmitResponse{}, err
	}
	return result, nil
}
