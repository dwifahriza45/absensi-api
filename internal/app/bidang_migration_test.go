package app

import (
	"context"
	"os"
	"testing"
)

func TestBidangMigrationPreservesLegacyLabels(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	run := func(name string) {
		t.Helper()
		sql, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	run("000010_instansi_to_mitra.down.sql")
	run("000009_remove_employee_daerah.down.sql")
	run("000008_bidang.down.sql")
	if _, err := db.Exec(ctx, `INSERT INTO employees(name,phone_hash,bidang) VALUES('First','first',' IT '),('Second','second','IT'),('Empty','empty','')`); err != nil {
		t.Fatal(err)
	}
	run("000008_bidang.up.sql")
	run("000009_remove_employee_daerah.up.sql")
	run("000010_instansi_to_mitra.up.sql")
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM bidang WHERE name='IT'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("dedup: %d %v", count, err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM employees e JOIN bidang b ON b.bidang_id=e.bidang_id WHERE b.name='IT'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("references: %d %v", count, err)
	}
	run("000010_instansi_to_mitra.down.sql")
	run("000009_remove_employee_daerah.down.sql")
	run("000008_bidang.down.sql")
	if err := db.QueryRow(ctx, `SELECT count(*) FROM employees WHERE bidang='IT'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("rollback: %d %v", count, err)
	}
	run("000008_bidang.up.sql")
	run("000009_remove_employee_daerah.up.sql")
	run("000010_instansi_to_mitra.up.sql")
}
