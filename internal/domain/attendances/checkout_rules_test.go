package attendances

import (
	"testing"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/shared"
)

func TestCheckOutRulesWithoutTimeGate(t *testing.T) {
	s := &Service{schedule: shared.Schedule{PhotoSessions: 3}}
	for _, hour := range []int{8, 14, 18} {
		now := time.Date(2026, 11, 1, hour, 0, 0, 0, time.UTC)
		for _, done := range []int{0, 2, 3} {
			allowed, reason := s.checkOutRules(now, Attendance{}, done)
			if !allowed || reason != (done < 3) {
				t.Fatalf("hour=%d done=%d allowed=%v reason=%v", hour, done, allowed, reason)
			}
		}
		if allowed, _ := s.checkOutRules(now, Attendance{CheckOutAt: &now}, 3); allowed {
			t.Fatal("already checked out must not be allowed")
		}
	}
}
