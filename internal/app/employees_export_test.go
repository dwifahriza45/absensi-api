package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/xuri/excelize/v2"
)

func TestEmployeeExportFiltersAndTextNIK(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	var user, mitra1, mitra2, bidang1, bidang2 string
	for _, item := range []struct {
		query  string
		target *string
	}{
		{`SELECT user_id FROM users LIMIT 1`, &user},
		{`INSERT INTO mitra(name) VALUES('Office One') RETURNING mitra_id`, &mitra1},
		{`INSERT INTO mitra(name) VALUES('Office Two') RETURNING mitra_id`, &mitra2},
		{`INSERT INTO bidang(name) VALUES('Division One') RETURNING bidang_id`, &bidang1},
		{`INSERT INTO bidang(name) VALUES('Division Two') RETURNING bidang_id`, &bidang2},
	} {
		if err := db.QueryRow(ctx, item.query).Scan(item.target); err != nil {
			t.Fatal(err)
		}
	}
	svc := employees.NewService(employees.NewRepository(db))
	active, inactive := true, false
	for _, row := range []employees.WriteRequest{
		{NIK: "0123456789012345", Name: "Export Alpha", MitraID: mitra1, BidangID: bidang1, IsActive: &active},
		{NIK: "0123456789012346", Name: "Export Beta", MitraID: mitra1, BidangID: bidang2, IsActive: &inactive},
		{NIK: "0123456789012347", Name: "=1+1", MitraID: mitra2, BidangID: bidang1, IsActive: &active},
	} {
		if _, err := svc.Save(ctx, "", row); err != nil {
			t.Fatal(err)
		}
	}
	tokens := token.NewManager("export-test-jwt", db)
	access, _, _, err := tokens.Issue(ctx, user, token.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerEmployeesRoutes(newRouteGroups(mux), employees.NewHandler(svc), middleware.NewGuards(tokens))
	for _, tc := range []struct {
		query      string
		auth       bool
		want, rows int
	}{
		{"", false, 401, 0}, {"", true, 200, 2}, {"?status=all", true, 200, 3},
		{"?status=inactive", true, 200, 1}, {"?status=all&q=081234567892", true, 200, 0},
		{"?status=all&mitra_id=" + mitra1, true, 200, 2},
		{"?status=all&bidang_id=" + bidang1, true, 200, 2},
		{"?q=Alpha", true, 200, 1},
		{"?q=0123456789012345", true, 200, 1},
		{"?q=Alpha&mitra_id=" + mitra1 + "&bidang_id=" + bidang1, true, 200, 1},
		{"?q=Alpha&mitra_id=" + mitra2, true, 200, 0},
		{"?status=inactive&mitra_id=" + mitra1 + "&bidang_id=" + bidang2, true, 200, 1},
		{"?q=not-found", true, 200, 0}, {"?status=invalid", true, 400, 0},
	} {
		r := httptest.NewRequest("GET", "/api/v1/admin/employees/export"+tc.query, nil)
		if tc.auth {
			r.AddCookie(&http.Cookie{Name: "access_token", Value: access})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: %d %s", tc.query, w.Code, w.Body.String())
		}
		if tc.want != 200 {
			continue
		}
		if w.Header().Get("Content-Disposition") != `attachment; filename="karyawan.xlsx"` {
			t.Fatal("missing download filename")
		}
		f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		rows, err := f.GetRows("Karyawan")
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != tc.rows+1 || len(rows[0]) != 5 || rows[0][0] != "NAMA LENGKAP" || rows[0][1] != "MITRA" {
			t.Fatalf("%s wrong export: %+v", tc.query, rows)
		}
		for i := range rows[1:] {
			cell, _ := excelize.CoordinatesToCellName(1, i+2)
			formula, err := f.GetCellFormula("Karyawan", cell)
			if err != nil || formula != "" {
				t.Fatal("name exported as formula")
			}
		}
		f.Close()
	}
}
