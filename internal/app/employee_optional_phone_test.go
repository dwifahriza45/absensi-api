package app

import (
	"context"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/shared"
	"testing"
)

func TestEmployeeOptionalPhonePersistence(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	var mitra, bidang string
	if err := db.QueryRow(ctx, `INSERT INTO mitra(name) VALUES('Optional contact') RETURNING mitra_id`).Scan(&mitra); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO bidang(name) VALUES('Optional contact') RETURNING bidang_id`).Scan(&bidang); err != nil {
		t.Fatal(err)
	}
	svc := employees.NewService(employees.NewRepository(db), shared.NewPhoneHasher("optional-contact"))
	ids := map[string]bool{}
	for _, nik := range []string{"0123456789012345", "0123456789012346"} {
		saved, err := svc.Save(ctx, "", employees.WriteRequest{Name: "No contact", NIK: nik, MitraID: mitra, BidangID: bidang})
		if err != nil {
			t.Fatal(err)
		}
		if saved.Phone != "" || saved.EmployeeID == "" || ids[saved.EmployeeID] {
			t.Fatalf("invalid saved employee: %+v", saved)
		}
		ids[saved.EmployeeID] = true
	}
	result, err := svc.Import(ctx, employees.ImportRequest{Rows: []employees.ImportRow{
		{Name: "Updated", NIK: "0123456789012345", Mitra: "Optional contact", Bidang: "Optional contact"},
		{Name: "New", NIK: "0123456789012347", Mitra: "Optional contact", Bidang: "Optional contact"},
	}})
	if err != nil || result.Updated != 1 || result.Created != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM employees WHERE phone_hash IS NULL`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("blank contacts=%d err=%v", count, err)
	}
}
