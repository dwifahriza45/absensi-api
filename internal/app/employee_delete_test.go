package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
)

func TestEmployeeHardDeleteOnlyWithoutAttendance(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	repo := employees.NewRepository(db)
	tokens := token.NewManager("delete-test-secret", db)
	id := newEmployee(t, db)
	access, refresh, _, err := tokens.Issue(ctx, id, token.RoleEmployee)
	if err != nil {
		t.Fatal(err)
	}
	svc := employees.NewService(repo)
	detail, err := svc.AdminDetail(ctx, id)
	if err != nil || !detail.CanDelete || detail.HasAttendance {
		t.Fatalf("new employee metadata: %+v %v", detail, err)
	}
	if err = repo.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.FindByID(ctx, id); !errors.Is(err, employees.ErrNotFound) {
		t.Fatalf("employee not deleted: %v", err)
	}
	if _, err = tokens.Parse(ctx, access); err == nil {
		t.Fatal("deleted employee access still works")
	}
	if _, _, _, err = tokens.Refresh(ctx, refresh, token.RoleEmployee); err == nil {
		t.Fatal("deleted employee refresh still works")
	}
	if err = repo.Delete(ctx, id); !errors.Is(err, employees.ErrNotFound) {
		t.Fatalf("repeat delete: %v", err)
	}

	id = newEmployee(t, db)
	if _, err = db.Exec(ctx, `INSERT INTO attendances(id,employee_id,work_date,check_in_at) VALUES(gen_random_uuid(),$1,'2026-11-01',now())`, id); err != nil {
		t.Fatal(err)
	}
	for _, active := range []bool{true, false} {
		if _, err = db.Exec(ctx, `UPDATE employees SET is_active=$2 WHERE employee_id=$1`, id, active); err != nil {
			t.Fatal(err)
		}
		detail, err = svc.AdminDetail(ctx, id)
		if err != nil || detail.CanDelete || !detail.HasAttendance || detail.DeleteReason == "" {
			t.Fatalf("history metadata: %+v %v", detail, err)
		}
		if err = repo.Delete(ctx, id); !errors.Is(err, employees.ErrHasAttendance) {
			t.Fatalf("history delete must conflict: %v", err)
		}
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM attendances WHERE employee_id=$1`, id).Scan(&count); err != nil || count != 1 {
		t.Fatal("attendance history changed")
	}
}

func TestEmployeeDeleteWaitsForConcurrentAttendance(t *testing.T) {
	db, other := concurrencyDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id := newEmployee(t, db)
	tx, err := other.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Rollback(tx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "attendance:"+id); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO attendances(id,employee_id,work_date,check_in_at) VALUES(gen_random_uuid(),$1,'2026-11-01',now())`, id); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- employees.NewRepository(db).Delete(ctx, id) }()
	select {
	case err = <-done:
		t.Fatalf("delete did not wait for attendance: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, employees.ErrHasAttendance) {
		t.Fatalf("committed attendance must prevent delete: %v", err)
	}
	if _, err = employees.NewRepository(db).FindByID(ctx, id); err != nil {
		t.Fatal("employee lost during concurrent check-in")
	}
}
