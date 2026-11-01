package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
)

func TestRemoveDaerahMigrationPreservesEmployeeAndAttendance(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	run := func(name string) {
		t.Helper()
		data, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	id := newEmployee(t, db)
	before, err := employees.NewRepository(db).FindByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	run("000010_instansi_to_mitra.down.sql")
	run("000009_remove_employee_daerah.down.sql")
	if _, err = db.Exec(ctx, `UPDATE employees SET daerah='Legacy Region' WHERE employee_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO attendances(id,employee_id,work_date,check_in_at) VALUES(gen_random_uuid(),$1,'2026-11-01',now())`, id); err != nil {
		t.Fatal(err)
	}
	run("000009_remove_employee_daerah.up.sql")
	run("000010_instansi_to_mitra.up.sql")
	var exists bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='employees' AND column_name='daerah')`).Scan(&exists); err != nil || exists {
		t.Fatal("daerah column still exists")
	}
	after, err := employees.NewRepository(db).FindByID(ctx, id)
	if err != nil || before.ID != after.ID || before.EmployeeID != after.EmployeeID || before.Name != after.Name || before.PhoneHash != after.PhoneHash || !after.HasAttendance {
		t.Fatalf("employee/history changed: %v", err)
	}
	var index string
	if err = db.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname=current_schema() AND indexname='idx_employees_filters'`).Scan(&index); err != nil || strings.Contains(index, "daerah") {
		t.Fatalf("old filter index: %s %v", index, err)
	}
}
