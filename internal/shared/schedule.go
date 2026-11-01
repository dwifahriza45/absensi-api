package shared

import (
	"fmt"
	"math"
	"time"
)

// Schedule adalah jadwal kerja dalam menit sejak 00.00 (480 = 08.00).
type Schedule struct {
	Start         int
	End           int
	BreakStart    int
	BreakEnd      int
	PhotoSessions int
}

// Session adalah satu rentang waktu foto hasil kerja.
type Session struct {
	No         int
	From       int
	To         int
	SpansBreak bool
}

// NewSchedule membuat Schedule dari jam "HH:MM".
func NewSchedule(start, end, breakStart, breakEnd string, photoSessions int) (Schedule, error) {
	values := make([]int, 4)
	for i, raw := range []string{start, end, breakStart, breakEnd} {
		minutes, err := ParseClock(raw)
		if err != nil {
			return Schedule{}, err
		}
		values[i] = minutes
	}

	s := Schedule{Start: values[0], End: values[1], BreakStart: values[2], BreakEnd: values[3], PhotoSessions: photoSessions}
	if !(s.Start < s.BreakStart && s.BreakStart <= s.BreakEnd && s.BreakEnd < s.End) {
		return Schedule{}, fmt.Errorf("jadwal tidak valid: %s-%s, istirahat %s-%s", start, end, breakStart, breakEnd)
	}
	return s, nil
}

// Sessions membagi jam kerja bersih (tanpa istirahat) rata menjadi beberapa sesi foto.
// Sesi yang melewati jam istirahat diperpanjang selama istirahat.
func (s Schedule) Sessions() []Session {
	work := (s.End - s.Start) - (s.BreakEnd - s.BreakStart)
	perSession := int(math.Round(float64(work) / float64(s.PhotoSessions)))

	sessions := make([]Session, 0, s.PhotoSessions)
	cursor := s.Start
	for i := 0; i < s.PhotoSessions; i++ {
		isLast := i == s.PhotoSessions-1
		session := Session{No: i + 1, From: cursor, To: cursor}
		remaining := perSession

		for (isLast || remaining > 0) && session.To < s.End {
			if session.To >= s.BreakStart && session.To < s.BreakEnd {
				session.To = s.BreakEnd
				session.SpansBreak = true
				continue
			}
			limit := s.End
			if session.To < s.BreakStart {
				limit = s.BreakStart
			}
			step := limit - session.To
			if !isLast && remaining < step {
				step = remaining
			}
			session.To += step
			remaining -= step
		}

		sessions = append(sessions, session)
		cursor = session.To
	}
	return sessions
}

// SessionByNo mencari sesi berdasarkan nomornya.
func (s Schedule) SessionByNo(no int) (Session, bool) {
	for _, session := range s.Sessions() {
		if session.No == no {
			return session, true
		}
	}
	return Session{}, false
}

// ParseClock mengubah "08:00" atau "08.00" menjadi menit sejak 00.00.
func ParseClock(raw string) (int, error) {
	var hour, minute int
	if _, err := fmt.Sscanf(raw, "%d:%d", &hour, &minute); err != nil {
		if _, err := fmt.Sscanf(raw, "%d.%d", &hour, &minute); err != nil {
			return 0, fmt.Errorf("format jam tidak valid: %q", raw)
		}
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("jam di luar rentang: %q", raw)
	}
	return hour*60 + minute, nil
}

// FormatClock mengubah menit menjadi "08.00".
func FormatClock(minutes int) string {
	return fmt.Sprintf("%02d.%02d", minutes/60, minutes%60)
}

// MinutesOf mengambil menit sejak 00.00 dari sebuah waktu.
func MinutesOf(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}
