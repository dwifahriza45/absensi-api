package attendances

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/haidarrasyad/absensi-api/internal/domain/photos"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

const (
	reviewAccuracyM = 100 // lokasi kurang akurat dari ini ditandai untuk ditinjau
	maxReasonLength = 200
	maxRangeDays    = 31
)

// PhotoSaver adalah kebutuhan absen terhadap domain foto.
type PhotoSaver interface {
	Save(ctx context.Context, kind photos.Kind, r io.Reader) (photos.Photo, error)
}

// ReportLister memberi daftar sesi foto hasil kerja yang sudah diterima.
type ReportLister interface {
	SubmittedSessions(ctx context.Context, attendanceID string) (map[int]time.Time, error)
}

// Service berisi aturan bisnis absen.
type Service struct {
	repo        Repository
	photos      PhotoSaver
	reports     ReportLister
	clock       shared.Clock
	schedule    shared.Schedule
	coordinator shared.MutationCoordinator
}

// NewService membuat Service absen.
func NewService(repo Repository, photos PhotoSaver, reports ReportLister, clock shared.Clock, schedule shared.Schedule, coordinator shared.MutationCoordinator) *Service {
	return &Service{repo: repo, photos: photos, reports: reports, clock: clock, schedule: schedule, coordinator: coordinator}
}

// Today mengembalikan status hari ini untuk beranda pegawai.
func (s *Service) Today(ctx context.Context, employeeID string) (TodayResponse, error) {
	now := s.clock.Now()
	resp := TodayResponse{Date: shared.DateString(now), Schedule: s.scheduleDTO()}

	attendance, err := s.repo.FindByEmployeeDate(ctx, employeeID, resp.Date)
	if errors.Is(err, ErrNotFound) {
		resp.Sessions = s.sessionStatuses(now, nil)
		return resp, nil
	}
	if err != nil {
		return resp, err
	}

	submitted, err := s.reports.SubmittedSessions(ctx, attendance.AttendanceID)
	if err != nil {
		return resp, err
	}

	dto := toDTO(attendance)
	resp.Attendance = &dto
	resp.Sessions = s.sessionStatuses(now, submitted)
	resp.CanCheckOut, resp.CheckOutReasonRequired = s.checkOutRules(now, attendance, len(submitted))
	return resp, nil
}

// CheckIn requires a captured selfie and valid location.
func (s *Service) checkIn(ctx context.Context, employeeID string, in CheckInInput) (AttendanceDTO, error) {
	if in.Selfie == nil {
		return AttendanceDTO{}, ErrSelfieRequired
	}
	if in.Location == nil {
		return AttendanceDTO{}, ErrLocationRequired
	}
	if err := validateLocation(in.Location); err != nil {
		return AttendanceDTO{}, err
	}

	now := s.clock.Now()
	date := shared.DateString(now)

	_, err := s.repo.FindByEmployeeDate(ctx, employeeID, date)
	if err == nil {
		return AttendanceDTO{}, ErrAlreadyCheckedIn
	}
	if !errors.Is(err, ErrNotFound) {
		return AttendanceDTO{}, err
	}

	attendance := Attendance{
		ID:              shared.NewID(),
		EmployeeID:      employeeID,
		WorkDate:        date,
		CheckInAt:       now,
		CheckInLocation: in.Location,
	}
	if in.Selfie != nil {
		photo, err := s.photos.Save(ctx, photos.KindSelfieIn, in.Selfie)
		if err != nil {
			return AttendanceDTO{}, err
		}
		attendance.CheckInSelfieID = &photo.PhotoID
	}
	attendance.CheckInFlag = needsReview(attendance.CheckInSelfieID, in.Location)

	created, err := s.repo.Create(ctx, attendance)
	if err != nil {
		return AttendanceDTO{}, err
	}
	return toDTO(created), nil
}

// CheckOut mencatat absen pulang.
func (s *Service) checkOut(ctx context.Context, employeeID string, in CheckOutInput) (AttendanceDTO, error) {
	if in.Selfie == nil {
		return AttendanceDTO{}, ErrSelfieRequired
	}
	if in.Location == nil {
		return AttendanceDTO{}, ErrLocationRequired
	}
	if err := validateLocation(in.Location); err != nil {
		return AttendanceDTO{}, err
	}

	now := s.clock.Now()
	attendance, err := s.todayAttendance(ctx, employeeID, now)
	if err != nil {
		return AttendanceDTO{}, err
	}
	if attendance.CheckOutAt != nil {
		return AttendanceDTO{}, ErrAlreadyCheckedOut
	}

	submitted, err := s.reports.SubmittedSessions(ctx, attendance.AttendanceID)
	if err != nil {
		return AttendanceDTO{}, err
	}
	canCheckOut, reasonRequired := s.checkOutRules(now, attendance, len(submitted))
	if !canCheckOut {
		return AttendanceDTO{}, ErrCheckOutTooEarly
	}

	reason := cleanReason(in.Reason)
	if reason == "" && attendance.EarlyLeaveReason != nil {
		reason = *attendance.EarlyLeaveReason
	}
	if reasonRequired && reason == "" {
		return AttendanceDTO{}, ErrReasonRequired
	}

	attendance.CheckOutAt = &now
	attendance.CheckOutLocation = in.Location
	if reason != "" {
		attendance.CheckOutReason = &reason
	}
	if in.Selfie != nil {
		photo, err := s.photos.Save(ctx, photos.KindSelfieOut, in.Selfie)
		if err != nil {
			return AttendanceDTO{}, err
		}
		attendance.CheckOutSelfieID = &photo.PhotoID
	}

	if err := s.repo.UpdateCheckOut(ctx, attendance); err != nil {
		return AttendanceDTO{}, err
	}
	return toDTO(attendance), nil
}

// RequestEarlyLeave mencatat permintaan pulang lebih awal. Setelah itu pegawai boleh absen pulang.
func (s *Service) requestEarlyLeave(ctx context.Context, employeeID string, req EarlyLeaveRequest) (AttendanceDTO, error) {
	reason := cleanReason(req.Reason)
	if reason == "" {
		return AttendanceDTO{}, ErrReasonRequired
	}

	now := s.clock.Now()
	attendance, err := s.todayAttendance(ctx, employeeID, now)
	if err != nil {
		return AttendanceDTO{}, err
	}
	if attendance.CheckOutAt != nil {
		return AttendanceDTO{}, ErrAlreadyCheckedOut
	}

	attendance.EarlyLeaveRequestedAt = &now
	attendance.EarlyLeaveReason = &reason
	if err := s.repo.UpdateEarlyLeave(ctx, attendance); err != nil {
		return AttendanceDTO{}, err
	}
	return toDTO(attendance), nil
}

// AdminList mengembalikan absen dalam rentang tanggal sesuai filter.
func (s *Service) AdminList(ctx context.Context, filter AdminFilter) ([]AdminItem, error) {
	if err := validateRange(filter.From, filter.To); err != nil {
		return nil, err
	}
	filter.Query = strings.TrimSpace(filter.Query)

	records, err := s.repo.ListForAdmin(ctx, filter)
	if err != nil {
		return nil, err
	}
	items := make([]AdminItem, len(records))
	for i, rec := range records {
		items[i] = toAdminItem(rec)
	}
	return items, nil
}

// Version mengembalikan waktu perubahan terakhir pada suatu tanggal.
// Admin memanggil ini berkala dan hanya mengambil data lengkap bila nilainya berubah.
func (s *Service) Version(ctx context.Context, date string) (VersionResponse, error) {
	if date == "" {
		date = shared.DateString(s.clock.Now())
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return VersionResponse{}, ErrInvalidDateRange
	}
	latest, err := s.repo.LatestChange(ctx, date)
	if err != nil {
		return VersionResponse{}, err
	}
	return VersionResponse{Version: latest.UnixMilli()}, nil
}

func (s *Service) todayAttendance(ctx context.Context, employeeID string, now time.Time) (Attendance, error) {
	attendance, err := s.repo.FindByEmployeeDate(ctx, employeeID, shared.DateString(now))
	if errors.Is(err, ErrNotFound) {
		return Attendance{}, ErrNotCheckedIn
	}
	return attendance, err
}

// checkOutRules tidak membatasi jam pulang.
// Alasan tetap wajib bila bukti laporan kerja belum lengkap.
func (s *Service) checkOutRules(_ time.Time, a Attendance, doneSessions int) (canCheckOut, reasonRequired bool) {
	if a.CheckOutAt != nil {
		return false, false
	}
	canCheckOut = true
	reasonRequired = doneSessions < s.schedule.PhotoSessions
	return canCheckOut, reasonRequired
}

func (s *Service) sessionStatuses(now time.Time, submitted map[int]time.Time) []SessionDTO {
	minute := shared.MinutesOf(now)
	sessions := s.schedule.Sessions()
	out := make([]SessionDTO, 0, len(sessions))

	for _, session := range sessions {
		dto := SessionDTO{No: session.No, From: shared.FormatClock(session.From), To: shared.FormatClock(session.To)}
		if receivedAt, ok := submitted[session.No]; ok {
			dto.Status = SessionDone
			dto.ReceivedAt = &receivedAt
		} else {
			switch {
			case minute < session.From:
				dto.Status = SessionUpcoming
			case minute >= session.To:
				dto.Status = SessionMissed
			default:
				dto.Status = SessionOpen
			}
		}
		out = append(out, dto)
	}
	return out
}

func (s *Service) scheduleDTO() ScheduleDTO {
	return ScheduleDTO{
		Start:      shared.FormatClock(s.schedule.Start),
		End:        shared.FormatClock(s.schedule.End),
		BreakStart: shared.FormatClock(s.schedule.BreakStart),
		BreakEnd:   shared.FormatClock(s.schedule.BreakEnd),
	}
}

func needsReview(selfieID *string, location *Location) bool {
	return selfieID == nil || location == nil || location.AccuracyM > reviewAccuracyM
}

func validateLocation(l *Location) error {
	if l == nil {
		return nil
	}
	if l.Lat < -90 || l.Lat > 90 || l.Lng < -180 || l.Lng > 180 || l.AccuracyM < 0 {
		return ErrInvalidLocation
	}
	return nil
}

func validateRange(from, to string) error {
	start, err1 := time.Parse(time.DateOnly, from)
	end, err2 := time.Parse(time.DateOnly, to)
	if err1 != nil || err2 != nil || end.Before(start) {
		return ErrInvalidDateRange
	}
	if days := int(end.Sub(start).Hours()/24) + 1; days > maxRangeDays {
		return ValidateMonthRange(from, to)
	}
	return nil
}

func cleanReason(reason string) string {
	reason = strings.Join(strings.Fields(reason), " ")
	if utf8.RuneCountInString(reason) > maxReasonLength {
		reason = string([]rune(reason)[:maxReasonLength])
	}
	return reason
}

func (s *Service) CheckIn(ctx context.Context, employeeID string, in CheckInInput) (AttendanceDTO, error) {
	var result AttendanceDTO
	err := s.coordinator.Within(ctx, "attendance:"+employeeID, func(ctx context.Context) error {
		var err error
		result, err = s.checkIn(ctx, employeeID, in)
		return err
	})
	if err != nil {
		return AttendanceDTO{}, err
	}
	return result, nil
}

func (s *Service) CheckOut(ctx context.Context, employeeID string, in CheckOutInput) (AttendanceDTO, error) {
	var result AttendanceDTO
	err := s.coordinator.Within(ctx, "attendance:"+employeeID, func(ctx context.Context) error {
		var err error
		result, err = s.checkOut(ctx, employeeID, in)
		return err
	})
	if err != nil {
		return AttendanceDTO{}, err
	}
	return result, nil
}

func (s *Service) RequestEarlyLeave(ctx context.Context, employeeID string, req EarlyLeaveRequest) (AttendanceDTO, error) {
	var result AttendanceDTO
	err := s.coordinator.Within(ctx, "attendance:"+employeeID, func(ctx context.Context) error {
		var err error
		result, err = s.requestEarlyLeave(ctx, employeeID, req)
		return err
	})
	if err != nil {
		return AttendanceDTO{}, err
	}
	return result, nil
}
