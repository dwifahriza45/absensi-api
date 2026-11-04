package app

import (
	"context"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"testing"
)

func TestEmployeeImportResolvesDropdownNamesToMasterIDs(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	var mitraID, bidangID string
	if err := db.QueryRow(ctx, `INSERT INTO mitra(name) VALUES('Dropdown Mitra') RETURNING mitra_id`).Scan(&mitraID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO bidang(name) VALUES('Dropdown Bidang') RETURNING bidang_id`).Scan(&bidangID); err != nil {
		t.Fatal(err)
	}
	svc := employees.NewService(employees.NewRepository(db))
	for i := 0; i < 2; i++ {
		result, err := svc.Import(ctx, employees.ImportRequest{Rows: []employees.ImportRow{{NIK: "0123456789012345", Name: "Dropdown Employee", Mitra: "Dropdown Mitra", Bidang: "Dropdown Bidang"}}})
		if err != nil || len(result.Skipped) > 0 {
			t.Fatalf("import failed: %+v %v", result, err)
		}
		var actualMitra, actualBidang string
		if err := db.QueryRow(ctx, `SELECT mitra_id,bidang_id FROM employees WHERE name='Dropdown Employee'`).Scan(&actualMitra, &actualBidang); err != nil || actualMitra != mitraID || actualBidang != bidangID {
			t.Fatalf("wrong master references: %v", err)
		}
		if i == 1 && result.Updated != 1 {
			t.Fatal("reimport must update the existing employee")
		}
	}
}
