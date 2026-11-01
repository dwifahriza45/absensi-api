package app

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestMitraRenamePreservesReferencesAndCounter(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	run := func(name string) {
		t.Helper()
		sql, err := os.ReadFile("../../migrations/000010_instansi_to_mitra." + name + ".sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	employee := newEmployee(t, db)
	var uuid, code string
	if err := db.QueryRow(ctx, `INSERT INTO mitra(name,created_at) VALUES('Rename retained','2020-01-02') RETURNING id,mitra_id`).Scan(&uuid, &code); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE employees SET mitra_id=$1 WHERE employee_id=$2`, code, employee); err != nil {
		t.Fatal(err)
	}
	// Deleted allocations must remain consumed across upgrades and rollbacks.
	if _, err := db.Exec(ctx, `INSERT INTO mitra(name,created_at) VALUES('Deleted allocation','2020-01-02'); DELETE FROM mitra WHERE name='Deleted allocation'`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		run("down")
		var old string
		if err := db.QueryRow(ctx, `SELECT i.instansi_id FROM instansi i JOIN employees e USING(instansi_id) WHERE i.id=$1 AND e.employee_id=$2`, uuid, employee).Scan(&old); err != nil || old != strings.Replace(code, "MTR-", "INT-", 1) {
			t.Fatalf("rollback reference: %s %v", old, err)
		}
		run("up")
		var restored string
		if err := db.QueryRow(ctx, `SELECT m.mitra_id FROM mitra m JOIN employees e USING(mitra_id) WHERE m.id=$1 AND e.employee_id=$2`, uuid, employee).Scan(&restored); err != nil || restored != code {
			t.Fatalf("upgrade reference: %s %v", restored, err)
		}
	}
	var next string
	if err := db.QueryRow(ctx, `INSERT INTO mitra(name,created_at) VALUES('Next allocation','2020-01-02') RETURNING mitra_id`).Scan(&next); err != nil || next != "MTR-20200102-000003" {
		t.Fatalf("counter reset: %s %v", next, err)
	}
	if _, err := db.Exec(ctx, `DELETE FROM mitra WHERE id=$1`, uuid); err == nil {
		t.Fatal("referenced mitra was deleted")
	}
}
