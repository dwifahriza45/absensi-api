package app

import (
	"bytes"
	"github.com/haidarrasyad/absensi-api/internal/domain/bidang"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/domain/mitra"
	"github.com/haidarrasyad/absensi-api/internal/shared"
	"github.com/xuri/excelize/v2"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestImportTemplateHeaderStyles(t *testing.T) {
	db, _ := concurrencyDB(t)
	svc := employees.NewService(employees.NewRepository(db), shared.NewPhoneHasher("template-test"))
	for _, tc := range []struct {
		sheet   string
		cells   []string
		handler http.HandlerFunc
	}{
		{"Karyawan", []string{"A1", "B1", "C1", "D1"}, employees.NewHandler(svc).Template},
		{"Mitra", []string{"A1"}, mitra.NewHandler(nil).Template},
		{"Bidang", []string{"A1"}, bidang.NewHandler(nil).Template},
	} {
		t.Run(tc.sheet, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler(w, httptest.NewRequest("GET", "/import/template", nil))
			if w.Code != 200 {
				t.Fatalf("template failed: %d", w.Code)
			}
			f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			for _, cell := range tc.cells {
				id, err := f.GetCellStyle(tc.sheet, cell)
				if err != nil {
					t.Fatal(err)
				}
				style, err := f.GetStyle(id)
				if err != nil {
					t.Fatal(err)
				}
				if style.Font == nil || !style.Font.Bold || style.Font.Color != "FFFFFF" || len(style.Fill.Color) != 1 || style.Fill.Color[0] != "0D365B" || style.Alignment == nil || style.Alignment.Horizontal != "center" {
					t.Fatalf("%s missing template header styling: %+v", cell, style)
				}
			}
		})
	}
}
