package app

import (
	"context"
	"errors"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/domain/auth"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

func TestEmployeeNIKCredentialsAndImportIdentity(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	hasher := shared.NewPhoneHasher("nik-test")
	svc := employees.NewService(employees.NewRepository(db), hasher)
	var mitra, bidang string
	if err := db.QueryRow(ctx, `INSERT INTO mitra(name) VALUES('NIK Mitra') RETURNING mitra_id`).Scan(&mitra); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO bidang(name) VALUES('NIK Bidang') RETURNING bidang_id`).Scan(&bidang); err != nil {
		t.Fatal(err)
	}
	req := employees.WriteRequest{Name: "Nama Sama", NIK: "0123456789012345", Phone: "081234567890", MitraID: mitra, BidangID: bidang}
	first, err := svc.Save(ctx, "", req)
	if err != nil {
		t.Fatal(err)
	}
	req.Phone = "081234567891"
	if _, err := svc.Save(ctx, "", req); err == nil {
		t.Fatal("duplicate NIK accepted")
	}
	req.NIK = "0123456789012346"
	second, err := svc.Save(ctx, "", req)
	if err != nil {
		t.Fatal(err)
	}
	tokens := token.NewManager("nik-jwt", db)
	login := auth.NewService(svc, auth.NewRepository(db), tokens, hasher, 3000)
	// Same names are resolved using the unique NIK, rather than choosing a name match.
	for _, employee := range []employees.Response{first, second} {
		session, err := login.LoginEmployee(ctx, employee.EmployeeID, auth.EmployeeLoginRequest{Fullname: " nama   sama ", NIK: employee.NIK})
		if err != nil || session.Employee.EmployeeID != employee.EmployeeID {
			t.Fatalf("wrong identity: %v", err)
		}
		if session.Employee.NIK != "" {
			t.Fatal("NIK exposed in employee profile")
		}
	}
	for _, request := range []auth.EmployeeLoginRequest{{Fullname: "Other Name", NIK: first.NIK}, {Fullname: first.Name, NIK: "081234567890"}} {
		if _, err := login.LoginEmployee(ctx, "invalid", request); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Fatalf("invalid credentials accepted: %v", err)
		}
	}
	session, err := login.LoginEmployee(ctx, "before-edit", auth.EmployeeLoginRequest{Fullname: first.Name, NIK: first.NIK})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO attendances(id,employee_id,work_date,check_in_at) VALUES(gen_random_uuid(),$1,'2026-11-01',now())`, first.EmployeeID); err != nil {
		t.Fatal(err)
	}
	// Exported immutable ID lets import change NIK without creating a new employee.
	row := employees.ImportRow{EmployeeID: first.EmployeeID, NIK: "0123456789012347", Name: first.Name, Phone: first.Phone, Mitra: "NIK Mitra", Bidang: "NIK Bidang"}
	result, err := svc.Import(ctx, employees.ImportRequest{Rows: []employees.ImportRow{row}})
	if err != nil || result.Updated != 1 || result.Created != 0 {
		t.Fatalf("NIK edit import: %+v %v", result, err)
	}
	if _, err := tokens.Parse(ctx, session.Token); err == nil {
		t.Fatal("NIK change kept access session")
	}
	if _, _, _, err := tokens.Refresh(ctx, session.RefreshToken, token.RoleEmployee); err == nil {
		t.Fatal("NIK change kept refresh session")
	}
	if _, err := login.LoginEmployee(ctx, "old-nik", auth.EmployeeLoginRequest{Fullname: first.Name, NIK: first.NIK}); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatal("old NIK still accepted")
	}
	updated, err := svc.AdminDetail(ctx, first.EmployeeID)
	if err != nil || updated.NIK != row.NIK || !updated.HasAttendance {
		t.Fatalf("identity/history lost: %+v %v", updated, err)
	}
	if _, err := login.LoginEmployee(ctx, "new-nik", auth.EmployeeLoginRequest{Fullname: first.Name, NIK: row.NIK}); err != nil {
		t.Fatal(err)
	}
	// Changing to somebody else's NIK fails atomically.
	row.NIK = second.NIK
	if _, err := svc.Import(ctx, employees.ImportRequest{Rows: []employees.ImportRow{row}}); err == nil {
		t.Fatal("conflicting NIK accepted by import")
	}
	updated, err = svc.AdminDetail(ctx, first.EmployeeID)
	if err != nil || updated.NIK != "0123456789012347" {
		t.Fatal("failed import changed NIK")
	}
	// A pre-migration employee can acquire NIK without losing its stable ID.
	legacyID := newEmployee(t, db)
	if _, err := db.Exec(ctx, `UPDATE employees SET phone_hash=$2 WHERE employee_id=$1`, legacyID, hasher.Hash("081234567892")); err != nil {
		t.Fatal(err)
	}
	row = employees.ImportRow{NIK: "0123456789012348", Name: "Legacy", Phone: "081234567892", Mitra: "NIK Mitra", Bidang: "NIK Bidang"}
	result, err = svc.Import(ctx, employees.ImportRequest{Rows: []employees.ImportRow{row}})
	if err != nil || result.Updated != 1 {
		t.Fatalf("legacy fill: %+v %v", result, err)
	}
	legacy, err := svc.AdminDetail(ctx, legacyID)
	if err != nil || legacy.NIK != row.NIK {
		t.Fatal("legacy ID changed")
	}
	// Editing NIK directly is supported too.
	req = employees.WriteRequest{Name: second.Name, NIK: "0123456789012349", MitraID: mitra, BidangID: bidang}
	if _, err := svc.Save(ctx, second.EmployeeID, req); err != nil {
		t.Fatal(err)
	}
	req.NIK = "0123456789012347"
	if _, err := svc.Save(ctx, second.EmployeeID, req); err == nil {
		t.Fatal("duplicate NIK accepted by edit")
	}
}
