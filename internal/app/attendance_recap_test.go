package app

import (
	"bytes"
	"context"
	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/attendances"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAttendanceRecapFiltersAndPDF(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	aID, bID := newEmployee(t, db), newEmployee(t, db)
	if _, err := db.Exec(ctx, `UPDATE employees SET name=CASE WHEN employee_id=$1 THEN 'Rekap Alpha' ELSE 'Rekap Beta' END WHERE employee_id IN ($1,$2)`, aID, bID); err != nil {
		t.Fatal(err)
	}

	var mitra, bidang string
	if err := db.QueryRow(ctx, `INSERT INTO mitra(name) VALUES('Rekap Mitra') RETURNING mitra_id`).Scan(&mitra); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO bidang(name) VALUES('Rekap Bidang') RETURNING bidang_id`).Scan(&bidang); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE employees SET mitra_id=$1,bidang_id=$2 WHERE employee_id=$3`, mitra, bidang, aID); err != nil {
		t.Fatal(err)
	}
	a, reports, p := services(t, db, newTestStore(t), database.NewCoordinator(db))
	if _, err := a.CheckIn(ctx, aID, attendances.CheckInInput{Selfie: bytes.NewReader(testPNG), Location: &attendances.Location{Lat: -6.9, Lng: 106.9, AccuracyM: 10}}); err != nil {
		t.Fatal(err)
	}
	if _, err := reports.Submit(ctx, aID, 3, bytes.NewReader(testPNG)); err != nil {
		t.Fatal(err)
	}
	recap := attendances.NewRecap(a, recapEmployees(employees.NewRepository(db)), p)

	mux := http.NewServeMux()
	registerAttendancesRoutes(newRouteGroups(mux), attendances.NewHandler(a, recap), middleware.NewGuards(token.NewManager("report-test", db)))
	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest("GET", "/api/v1/admin/attendances/recap?from=2026-10-31&to=2026-10-31", nil))
	if unauthorized.Code != 401 {
		t.Fatal("PDF export exposed without admin session")
	}
	for _, scope := range []string{"", "&mitra=Rekap+Mitra", "&bidang=Rekap+Bidang", "&mitra=%20&bidang=Rekap+Bidang"} {
		w := httptest.NewRecorder()
		recap.ServeHTTP(w, httptest.NewRequest("GET", "/recap?from=2026-10-31&to=2026-10-31"+scope, nil))
		if w.Code != 400 || !strings.Contains(w.Body.String(), "Silakan pilih Mitra dan Bidang") {
			t.Fatal("required export scope bypassed")
		}
	}
	for _, tc := range []struct {
		query         string
		status, pages int
		hasBeta       bool
	}{
		{"from=2026-10-30&to=2026-10-31&include_absent=true", 200, 2, false},
		{"from=2026-10-30&to=2026-10-31&q=Alpha", 200, 2, false},
		{"from=2026-10-30&to=2026-10-31&include_absent=true&mitra=Rekap+Mitra&bidang=Rekap+Bidang", 200, 2, false},
		{"from=2026-10-30&to=2026-10-31&status=hadir", 200, 1, false},
		{"from=2026-10-30&to=2026-10-31&status=belum", 200, 1, false},
		{"from=2026-09-01&to=2026-10-31", 400, 0, false},
		{"from=2026-10-31&to=2026-10-31&q=missing", 400, 0, false},
		{"from=2026-10-31&to=2026-10-31&status=invalid", 400, 0, false},
	} {
		w := httptest.NewRecorder()
		recap.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/admin/attendances/recap?"+tc.query+"&mitra=Rekap+Mitra&bidang=Rekap+Bidang", nil))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.query, w.Code, w.Body.String())
		}
		if tc.status != 200 {
			continue
		}
		data := w.Body.Bytes()
		if !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.HasSuffix(data, []byte("%%EOF\n")) {
			t.Fatal("incomplete PDF")
		}
		if pages := bytes.Count(data, []byte("/Type /Page /Parent")); pages != tc.pages {
			t.Fatalf("%s: pages=%d want=%d", tc.query, pages, tc.pages)
		}
		if bytes.Contains(data, []byte("Rekap Beta")) != tc.hasBeta {
			t.Fatal("employee filter mismatch")
		}
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/pdf") {
			t.Fatal("wrong download type")
		}
		if tc.pages == 4 && os.Getenv("PDF_VERIFY_PATH") != "" {
			if err := os.WriteFile(os.Getenv("PDF_VERIFY_PATH"), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}
