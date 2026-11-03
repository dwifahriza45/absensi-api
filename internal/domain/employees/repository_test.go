package employees

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/domain/attendances"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMitraMigrationAndImport(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	schema := fmt.Sprintf("test_mitra_%d", time.Now().UnixNano())
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrate := func(file string) {
		t.Helper()
		sql, err := os.ReadFile("../../../migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	migrate("000001_init.up.sql")
	migrate("000002_admin_sessions.up.sql")
	_, err = db.Exec(ctx, `INSERT INTO employees(name,instansi,phone_hash) VALUES
 ('Legacy One',' Office A ','legacy1'),('Legacy Two','Office A','legacy2'),('No Office','','legacy3')`)
	if err != nil {
		t.Fatal(err)
	}
	migrate("000003_instansi.up.sql")
	migrate("000004_public_ids.up.sql")
	migrate("000006_business_key_references.up.sql")
	migrate("000007_employee_phone_encryption.up.sql")
	migrate("000008_bidang.up.sql")
	migrate("000009_remove_employee_daerah.up.sql")
	migrate("000010_instansi_to_mitra.up.sql")
	migrate("000011_employee_nik.up.sql")
	repo := NewRepository(db)
	list, err := repo.List(ctx, ListFilter{Mitra: "Office A"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].EmployeeID == "" || list[0].MitraID == "" || list[0].MitraID != list[1].MitraID {
		t.Fatalf("backfill: %+v", list)
	}
	oldID := list[0].MitraID
	imported, err := repo.Import(ctx, []Employee{
		{NIK: "0123456789012345", Name: "Imported One", Mitra: "Office A", PhoneHash: "new1"},
		{NIK: "0123456789012346", Name: "Imported Two", Mitra: "Office B", PhoneHash: "new2"},
		{NIK: "0123456789012347", Name: "Imported Three", PhoneHash: "new3"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Created != 3 {
		t.Fatalf("import: %+v", imported)
	}
	one, err := repo.FindByName(ctx, "Imported One")
	if err != nil || one.MitraID != oldID {
		t.Fatalf("reuse institution: %+v %v", one, err)
	}
	byID, err := repo.FindByID(ctx, one.EmployeeID)
	if err != nil || byID.Mitra != "Office A" {
		t.Fatalf("lookup: %+v %v", byID, err)
	}
	found, err := repo.Search(ctx, "Imported", 4)
	if err != nil || len(found) != 3 {
		t.Fatalf("search: %+v %v", found, err)
	}
	opts, err := repo.FilterOptions(ctx)
	if err != nil || len(opts.Mitra) != 2 {
		t.Fatalf("options: %+v %v", opts, err)
	}
	_, err = repo.Import(ctx, []Employee{{NIK: "0123456789012345", Name: "Imported One", Mitra: "Office B", PhoneHash: "new1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	one, err = repo.FindByName(ctx, "Imported One")
	if err != nil || one.MitraID == oldID || one.Mitra != "Office B" || one.EmployeeID != byID.EmployeeID {
		t.Fatalf("reassignment: %+v %v", one, err)
	}
	blank, err := repo.FindByName(ctx, "Imported Three")
	if err != nil || blank.MitraID != "" || blank.Mitra != "" {
		t.Fatalf("optional institution: %+v %v", blank, err)
	}
	if _, err = db.Exec(ctx, `DELETE FROM mitra WHERE id=$1`, oldID); err == nil {
		t.Fatal("deletion of referenced institution allowed")
	}
	if _, err = db.Exec(ctx, `UPDATE employees SET mitra_id='00000000-0000-0000-0000-000000000000' WHERE id=$1`, one.ID); err == nil {
		t.Fatal("invalid institution reference accepted")
	}

	if _, err = db.Exec(ctx, `INSERT INTO attendances(id,employee_id,work_date,check_in_at) VALUES(gen_random_uuid(),$1,'2026-10-31',now())`, one.EmployeeID); err != nil {
		t.Fatal(err)
	}
	report, err := attendances.NewRepository(db).ListForAdmin(ctx, attendances.AdminFilter{From: "2026-10-31", To: "2026-10-31", Mitra: "Office B"})
	if err != nil || len(report) != 1 || report[0].Mitra != "Office B" {
		t.Fatalf("attendance institution join: %+v %v", report, err)
	}
	migrate("000011_employee_nik.down.sql")
	migrate("000010_instansi_to_mitra.down.sql")
	migrate("000009_remove_employee_daerah.down.sql")
	migrate("000008_bidang.down.sql")
	migrate("000007_employee_phone_encryption.down.sql")
	migrate("000006_business_key_references.down.sql")
	migrate("000004_public_ids.down.sql")
	migrate("000003_instansi.down.sql")
	var restored string
	if err = db.QueryRow(ctx, `SELECT instansi FROM employees WHERE id=$1`, one.ID).Scan(&restored); err != nil || restored != "Office B" {
		t.Fatalf("rollback data: %s %v", restored, err)
	}
	migrate("000003_instansi.up.sql")
	migrate("000004_public_ids.up.sql")
	migrate("000006_business_key_references.up.sql")
	migrate("000007_employee_phone_encryption.up.sql")
	migrate("000008_bidang.up.sql")
	migrate("000009_remove_employee_daerah.up.sql")
	migrate("000010_instansi_to_mitra.up.sql")
	migrate("000011_employee_nik.up.sql")
	if _, err = repo.List(ctx, ListFilter{}); err != nil {
		t.Fatal(err)
	}
}
