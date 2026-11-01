package shared

import "testing"

func TestScheduleSessions(t *testing.T) {
	schedule, err := NewSchedule("08:00", "17:00", "12:00", "13:00", 3)
	if err != nil {
		t.Fatalf("NewSchedule: %v", err)
	}

	want := []struct {
		from, to string
		spans    bool
	}{
		{"08.00", "10.40", false},
		{"10.40", "14.20", true},
		{"14.20", "17.00", false},
	}

	got := schedule.Sessions()
	if len(got) != len(want) {
		t.Fatalf("jumlah sesi = %d, mau %d", len(got), len(want))
	}
	for i, session := range got {
		if FormatClock(session.From) != want[i].from || FormatClock(session.To) != want[i].to || session.SpansBreak != want[i].spans {
			t.Errorf("sesi %d = %s-%s (istirahat %v), mau %s-%s (%v)",
				session.No, FormatClock(session.From), FormatClock(session.To), session.SpansBreak,
				want[i].from, want[i].to, want[i].spans)
		}
	}
}

func TestParseClock(t *testing.T) {
	cases := map[string]int{"08:00": 480, "17.30": 1050, "00:05": 5}
	for raw, want := range cases {
		got, err := ParseClock(raw)
		if err != nil || got != want {
			t.Errorf("ParseClock(%q) = %d, %v; mau %d", raw, got, err, want)
		}
	}
	if _, err := ParseClock("25:00"); err == nil {
		t.Error("ParseClock(25:00) seharusnya error")
	}
}
