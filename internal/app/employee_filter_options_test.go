package app

import (
	"context"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
)

func TestEmployeeFilterOptionsIncludeUnassignedMasters(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	repo := employees.NewRepository(db)
	var mitra, bidang string
	if err := db.QueryRow(ctx, `INSERT INTO mitra(name) VALUES('Only Inactive Employee') RETURNING mitra_id`).Scan(&mitra); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO bidang(name) VALUES('Only Inactive Employee') RETURNING bidang_id`).Scan(&bidang); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO mitra(name) VALUES('Unassigned Institution'); INSERT INTO bidang(name) VALUES('Unassigned Division')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO employees(name,phone_hash,mitra_id,bidang_id,is_active) VALUES('Inactive Employee','filter-test',$1,$2,false)`, mitra, bidang); err != nil {
		t.Fatal(err)
	}
	options, err := repo.FilterOptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Mitra) != 2 || len(options.Bidang) != 2 {
		t.Fatalf("missing master options: %+v", options)
	}
	if options.Mitra[1] != "Unassigned Institution" || options.Bidang[1] != "Unassigned Division" {
		t.Fatalf("unassigned masters omitted: %+v", options)
	}
}
