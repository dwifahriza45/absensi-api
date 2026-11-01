package attendances

import (
	"net/http/httptest"
	"testing"
)

func TestMonthRange(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		valid bool
	}{
		{"2026-10-01", "2026-10-31", true}, {"2026-02-01", "2026-02-28", true}, {"2026-10-01", "2026-11-01", false}, {"2026-11-02", "2026-11-01", false}, {"invalid", "2026-11-01", false},
	} {
		if (ValidateMonthRange(tc.a, tc.b) == nil) != tc.valid {
			t.Fatalf("bad range validation: %+v", tc)
		}
	}
}
func TestRecapAdmissionRejectsParallelExport(t *testing.T) {
	s := NewRecap(nil, nil, nil)
	s.gate <- struct{}{}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/recap", nil))
	if w.Code != 429 {
		t.Fatal("parallel PDF export was admitted")
	}
}
