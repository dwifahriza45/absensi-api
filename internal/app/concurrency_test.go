package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/domain/attendances"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/domain/photos"
	workreports "github.com/haidarrasyad/absensi-api/internal/domain/work_reports"
	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/pkg/database"
	"github.com/haidarrasyad/absensi-api/internal/pkg/storage"
	"github.com/haidarrasyad/absensi-api/internal/shared"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC) }

var testPNG, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")

func concurrencyDB(t *testing.T) (*pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for concurrent transaction tests")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("test_concurrent_%d", time.Now().UnixNano())
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		root.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); root.Close() })
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.MaxConns = 32
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	other, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(other.Close)
	// Install pgcrypto once outside the test schema so concurrent packages don't race extension creation.
	if _, err = root.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pgcrypto"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000001_init.up.sql", "000002_admin_sessions.up.sql", "000003_instansi.up.sql", "000004_public_ids.up.sql", "000005_employee_login_index.up.sql", "000006_business_key_references.up.sql", "000007_employee_phone_encryption.up.sql", "000008_bidang.up.sql", "000009_remove_employee_daerah.up.sql", "000010_instansi_to_mitra.up.sql", "000011_employee_nik.up.sql"} {
		sql, err := os.ReadFile(filepath.Join("../../migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	return db, other
}
func newEmployee(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := db.QueryRow(context.Background(), `INSERT INTO employees(name,phone_hash) VALUES('Concurrent',$1) RETURNING employee_id`, shared.NewID()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func services(t *testing.T, db *pgxpool.Pool, store *storage.Local, coordinator shared.MutationCoordinator) (*attendances.Service, *workreports.Service, *photos.Service) {
	t.Helper()
	schedule, err := shared.NewSchedule("08:00", "17:00", "12:00", "13:00", 3)
	if err != nil {
		t.Fatal(err)
	}
	p := photos.NewService(photos.NewRepository(db), store, fixedClock{})
	a := attendances.NewRepository(db)
	r := workreports.NewRepository(db)
	return attendances.NewService(a, p, r, fixedClock{}, schedule, coordinator), workreports.NewService(r, p, attendanceLookup{repo: a}, fixedClock{}, schedule, coordinator), p
}
func newTestStore(t *testing.T) *storage.Local {
	t.Helper()
	s, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func rowCount(t *testing.T, db *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestConcurrentAttendanceMutations(t *testing.T) {
	db, other := concurrencyDB(t)
	store := newTestStore(t)
	ctx := context.Background()
	id := newEmployee(t, db)
	a1, r1, _ := services(t, db, store, database.NewCoordinator(db))
	a2, r2, _ := services(t, other, store, database.NewCoordinator(other))
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for n := 0; n < 24; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			a := a1
			if n%2 == 1 {
				a = a2
			}
			_, err := a.CheckIn(ctx, id, attendances.CheckInInput{Location: &attendances.Location{Lat: -6.9, Lng: 106.9, AccuracyM: 10}, Selfie: bytes.NewReader(testPNG)})
			errs <- err
		}(n)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, attendances.ErrAlreadyCheckedIn) {
			t.Fatal(err)
		}
	}
	if success != 1 || rowCount(t, db, "attendances") != 1 || rowCount(t, db, "photos") != 1 {
		t.Fatalf("checkin success=%d photos=%d", success, rowCount(t, db, "photos"))
	}
	errs = make(chan error, 24)
	ids := make(chan string, 24)
	for n := 0; n < 24; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r := r1
			if n%2 == 1 {
				r = r2
			}
			result, err := r.Submit(ctx, id, 1, bytes.NewReader(testPNG))
			errs <- err
			ids <- result.ID
		}(n)
	}
	wg.Wait()
	close(errs)
	close(ids)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	same := ""
	for id := range ids {
		if same != "" && same != id {
			t.Fatal("duplicate report returned different records")
		}
		same = id
	}
	if rowCount(t, db, "work_reports") != 1 || rowCount(t, db, "photos") != 2 {
		t.Fatal("duplicate reports or orphan photo metadata")
	}
	if _, err := a1.RequestEarlyLeave(ctx, id, attendances.EarlyLeaveRequest{Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	errs = make(chan error, 24)
	for n := 0; n < 24; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			a := a1
			if n%2 == 1 {
				a = a2
			}
			_, err := a.CheckOut(ctx, id, attendances.CheckOutInput{Location: &attendances.Location{Lat: -6.9, Lng: 106.9, AccuracyM: 10}, Selfie: bytes.NewReader(testPNG), Reason: "test"})
			errs <- err
		}(n)
	}
	wg.Wait()
	close(errs)
	success = 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, attendances.ErrAlreadyCheckedOut) {
			t.Fatal(err)
		}
	}
	if success != 1 || rowCount(t, db, "photos") != 3 {
		t.Fatalf("checkout success=%d photos=%d", success, rowCount(t, db, "photos"))
	}
}

type gateCoordinator struct {
	base             shared.MutationCoordinator
	entered, release chan struct{}
}

func (g gateCoordinator) Within(ctx context.Context, key string, fn func(context.Context) error) error {
	return g.base.Within(ctx, key, func(ctx context.Context) error {
		close(g.entered)
		select {
		case <-g.release:
			return fn(ctx)
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}
func TestReportVersusCheckoutOrdering(t *testing.T) {
	for _, first := range []string{"report", "checkout"} {
		t.Run(first, func(t *testing.T) {
			db, other := concurrencyDB(t)
			ctx := context.Background()
			store := newTestStore(t)
			id := newEmployee(t, db)
			a, r, _ := services(t, db, store, database.NewCoordinator(db))
			if _, err := a.CheckIn(ctx, id, attendances.CheckInInput{Selfie: bytes.NewReader(testPNG), Location: &attendances.Location{Lat: -6.9, Lng: 106.9, AccuracyM: 10}}); err != nil {
				t.Fatal(err)
			}
			if _, err := a.RequestEarlyLeave(ctx, id, attendances.EarlyLeaveRequest{Reason: "test"}); err != nil {
				t.Fatal(err)
			}
			gate := gateCoordinator{database.NewCoordinator(other), make(chan struct{}), make(chan struct{})}
			ga, gr, _ := services(t, other, store, gate)
			firstDone, secondDone := make(chan error, 1), make(chan error, 1)
			if first == "report" {
				go func() { _, err := gr.Submit(ctx, id, 1, bytes.NewReader(testPNG)); firstDone <- err }()
			} else {
				go func() {
					_, err := ga.CheckOut(ctx, id, attendances.CheckOutInput{Selfie: bytes.NewReader(testPNG), Location: &attendances.Location{Lat: -6.9, Lng: 106.9, AccuracyM: 10}, Reason: "test"})
					firstDone <- err
				}()
			}
			select {
			case <-gate.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("first request did not acquire lock")
			}
			if first == "report" {
				go func() {
					_, err := a.CheckOut(ctx, id, attendances.CheckOutInput{Selfie: bytes.NewReader(testPNG), Location: &attendances.Location{Lat: -6.9, Lng: 106.9, AccuracyM: 10}, Reason: "test"})
					secondDone <- err
				}()
			} else {
				go func() { _, err := r.Submit(ctx, id, 1, bytes.NewReader(testPNG)); secondDone <- err }()
			}
			select {
			case err := <-secondDone:
				t.Fatalf("second operation bypassed lock: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			close(gate.release)
			if err := <-firstDone; err != nil {
				t.Fatal(err)
			}
			err := <-secondDone
			if first == "report" {
				if err != nil {
					t.Fatal(err)
				}
				if rowCount(t, db, "work_reports") != 1 {
					t.Fatal("report not committed")
				}
			} else {
				if !errors.Is(err, workreports.ErrAlreadyCheckedOut) {
					t.Fatalf("late report: %v", err)
				}
				if rowCount(t, db, "work_reports") != 0 || rowCount(t, db, "photos") != 2 {
					t.Fatal("report/photo accepted after checkout")
				}
			}
		})
	}
}
func TestRollbackCleansMetadataAndFile(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	store := newTestStore(t)
	coordinator := database.NewCoordinator(db)
	_, _, p := services(t, db, store, coordinator)
	want := errors.New("force rollback")
	var photo photos.Photo
	err := coordinator.Within(ctx, "rollback-test", func(ctx context.Context) error {
		var err error
		photo, err = p.Save(ctx, photos.KindSelfieIn, bytes.NewReader(testPNG))
		if err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	if rowCount(t, db, "photos") != 0 {
		t.Fatal("photo metadata escaped transaction")
	}
	if file, err := store.Open(ctx, photo.StorageKey); err == nil {
		file.Close()
		t.Fatal("rolled-back photo file remains")
	}

	cancelled, cancel := context.WithCancel(ctx)
	err = coordinator.Within(cancelled, "cancel-test", func(ctx context.Context) error {
		var saveErr error
		photo, saveErr = p.Save(ctx, photos.KindSelfieIn, bytes.NewReader(testPNG))
		if saveErr != nil {
			return saveErr
		}
		cancel()
		return ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request: %v", err)
	}
	if rowCount(t, db, "photos") != 0 {
		t.Fatal("cancelled request committed photo metadata")
	}
	if f, err := store.Open(ctx, photo.StorageKey); err == nil {
		f.Close()
		t.Fatal("cancelled request left photo file")
	}
	if err := coordinator.Within(ctx, "cancel-test", func(context.Context) error { return nil }); err != nil {
		t.Fatal("cancelled request leaked lock", err)
	}
	// Panic unwinding also rolls back and releases the lock; middleware can recover it.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()
		_ = coordinator.Within(ctx, "panic-test", func(ctx context.Context) error { panic("test panic") })
	}()
	if err := coordinator.Within(ctx, "panic-test", func(context.Context) error { return nil }); err != nil {
		t.Fatal("lock leaked after panic", err)
	}
}
func TestLockWaitIsBounded(t *testing.T) {
	db, other := concurrencyDB(t)
	ctx := context.Background()
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- database.NewCoordinator(db).Within(ctx, "held-test", func(context.Context) error { close(held); <-release; return nil })
	}()
	<-held
	err := database.NewCoordinator(other).Within(ctx, "held-test", func(context.Context) error { t.Error("callback entered while lock held"); return nil })
	close(release)
	if firstErr := <-done; firstErr != nil {
		t.Fatal(firstErr)
	}
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.Status != 503 {
		t.Fatalf("lock timeout not controlled: %v", err)
	}
	if err := database.NewCoordinator(other).Within(ctx, "held-test", func(context.Context) error { return nil }); err != nil {
		t.Fatal("lock not released", err)
	}
}
func TestConcurrentImportsAreWholeSnapshots(t *testing.T) {
	db, other := concurrencyDB(t)
	ctx := context.Background()
	start := make(chan struct{})
	errs := make(chan error, 2)
	for n, pool := range []*pgxpool.Pool{db, other} {
		go func(n int, pool *pgxpool.Pool) {
			<-start
			_, err := employees.NewRepository(pool).Import(ctx, []employees.Employee{
				{NIK: fmt.Sprintf("%016d", n*2+1), Name: fmt.Sprintf("set-%d-one", n), PhoneHash: fmt.Sprintf("hash-%d-one", n), Mitra: "B"},
				{NIK: fmt.Sprintf("%016d", n*2+2), Name: fmt.Sprintf("set-%d-two", n), PhoneHash: fmt.Sprintf("hash-%d-two", n), Mitra: "A"},
			}, true)
			errs <- err
		}(n, pool)
	}
	close(start)
	for n := 0; n < 2; n++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var count, sets int
	if err := db.QueryRow(ctx, `SELECT count(*),count(DISTINCT split_part(name,'-',2)) FROM employees WHERE is_active`).Scan(&count, &sets); err != nil || count != 2 || sets != 1 {
		t.Fatalf("mixed import snapshot: count=%d sets=%d err=%v", count, sets, err)
	}
}
