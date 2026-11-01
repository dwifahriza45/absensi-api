package employees

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/domain/attendances"
	"github.com/haidarrasyad/absensi-api/internal/domain/photos"
	workreports "github.com/haidarrasyad/absensi-api/internal/domain/work_reports"
	"github.com/haidarrasyad/absensi-api/internal/shared"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPublicIDGeneration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for public ID integration tests")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	schema := fmt.Sprintf("test_public_ids_%d", time.Now().UnixNano())
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
	for _, file := range []string{"000001_init.up.sql", "000002_admin_sessions.up.sql", "000003_instansi.up.sql"} {
		migrate(file)
	}
	// Legacy records retain UUIDs and relationships after backfill. Jakarta date is Jan 2.
	var employee, photo, mitra string
	stamp := "2019-01-01T18:00:00Z"
	if _, err = db.Exec(ctx, `UPDATE users SET created_at=$1`, stamp); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO instansi(name,created_at) VALUES('Legacy Office',$1) RETURNING id::text`, stamp).Scan(&mitra); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO employees(name,phone_hash,instansi_id,created_at) VALUES('Legacy','legacy',$1,$2) RETURNING id::text`, mitra, stamp).Scan(&employee); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO photos(id,kind,storage_key,content_type,size_bytes,created_at) VALUES(gen_random_uuid(),'work_report','legacy','image/png',1,$1) RETURNING id::text`, stamp).Scan(&photo); err != nil {
		t.Fatal(err)
	}
	var attendance string
	if err = db.QueryRow(ctx, `INSERT INTO attendances(id,employee_id,work_date,check_in_at,created_at) VALUES(gen_random_uuid(),$1,'2019-01-02',$2,$2) RETURNING id::text`, employee, stamp).Scan(&attendance); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO work_reports(id,attendance_id,session_no,photo_id,received_at) VALUES(gen_random_uuid(),$1,1,$2,$3)`, attendance, photo, stamp); err != nil {
		t.Fatal(err)
	}
	migrate("000004_public_ids.up.sql")
	for _, tc := range []struct{ table, column, want string }{
		{"users", "user_id", "USER-20190102-000001"},
		{"employees", "user_id", "USER-20190102-000002"},
		{"instansi", "instansi_id", "INT-20190102-000001"},
		{"attendances", "attendance_id", "ATT-20190102-000001"},
		{"photos", "photo_id", "PHO-20190102-000001"},
		{"work_reports", "work_report_id", "WRP-20190102-000001"},
	} {
		var got string
		if err = db.QueryRow(ctx, "SELECT "+tc.column+" FROM "+tc.table).Scan(&got); err != nil || got != tc.want {
			t.Fatalf("%s: %s want %s (%v)", tc.table, got, tc.want, err)
		}
	}
	// Populate every legacy reference plus active admin/employee sessions before upgrade.
	if _, err = db.Exec(ctx, `UPDATE attendances SET check_in_selfie_id=$1 WHERE id=$2`, photo, attendance); err != nil {
		t.Fatal(err)
	}
	rawRefresh := "legacy-refresh-for-migration-test"
	digest := sha256.Sum256([]byte(rawRefresh))
	var legacySession, adminUUID string
	if err = db.QueryRow(ctx, `SELECT id::text FROM users LIMIT 1`).Scan(&adminUUID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO sessions(subject,role,refresh_hash,expires_at,created_at) VALUES($1,'karyawan',$2,now()+interval '1 day',$3) RETURNING id::text`, employee, hex.EncodeToString(digest[:]), stamp).Scan(&legacySession); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO sessions(subject,role,refresh_hash,expires_at) VALUES($1,'admin','admin-hash',now()+interval '1 day'),(gen_random_uuid(),'karyawan','orphan-hash',now()+interval '1 day')`, adminUUID); err != nil {
		t.Fatal(err)
	}
	secret := "migration-test-secret-at-least-32-characters"
	legacyAccess, err := jwt.NewWithClaims(jwt.SigningMethodHS256, token.Claims{Role: token.RoleEmployee, RegisteredClaims: jwt.RegisteredClaims{ID: legacySession, Subject: employee, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}

	migrate("000006_business_key_references.up.sql")
	migrate("000007_employee_phone_encryption.up.sql")
	migrate("000008_bidang.up.sql")
	migrate("000009_remove_employee_daerah.up.sql")
	migrate("000010_instansi_to_mitra.up.sql")

	if _, err = token.NewManager(secret, db).Parse(ctx, legacyAccess); err == nil {
		t.Fatal("legacy UUID access unexpectedly accepted")
	}
	access, _, _, err := token.NewManager(secret, db).Refresh(ctx, rawRefresh, token.RoleEmployee)
	if err != nil {
		t.Fatal("legacy refresh lost during upgrade", err)
	}
	claims, err := token.NewManager(secret, db).Parse(ctx, access)
	if err != nil || claims.Subject != "EMP-20190102-000002" || !shared.IsPublicID(claims.ID, "SES") {
		t.Fatalf("new session claims: %+v %v", claims, err)
	}
	var sessionCount int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&sessionCount); err != nil || sessionCount != 2 {
		t.Fatalf("valid/orphan sessions: %d %v", sessionCount, err)
	}
	var joined int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM work_reports w
 JOIN attendances a ON a.attendance_id=w.attendance_id
 JOIN employees e ON e.employee_id=a.employee_id
 JOIN mitra i ON i.mitra_id=e.mitra_id
 JOIN photos p ON p.photo_id=w.photo_id AND p.photo_id=a.check_in_selfie_id`).Scan(&joined); err != nil || joined != 1 {
		t.Fatalf("migrated relations: %d %v", joined, err)
	}
	for _, tc := range []struct{ table, column, target string }{
		{"employees", "mitra_id", "mitra(mitra_id)"},
		{"attendances", "employee_id", "employees(employee_id)"},
		{"attendances", "check_in_selfie_id", "photos(photo_id)"},
		{"attendances", "check_out_selfie_id", "photos(photo_id)"},
		{"work_reports", "attendance_id", "attendances(attendance_id)"},
		{"work_reports", "photo_id", "photos(photo_id)"},
		{"sessions", "user_id", "users(user_id)"},
		{"sessions", "employee_id", "employees(employee_id)"},
	} {
		var definition string
		if err = db.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid=$1::regclass AND conname=$2`, tc.table, tc.table+"_"+tc.column+"_fkey").Scan(&definition); err != nil || !strings.Contains(definition, "REFERENCES "+tc.target) {
			t.Fatalf("FK %s.%s: %s %v", tc.table, tc.column, definition, err)
		}
	}
	if _, err = db.Exec(ctx, `UPDATE attendances SET employee_id=$1 WHERE attendance_id='ATT-20190102-000001'`, employee); err == nil {
		t.Fatal("UUID reference accepted in employee_id")
	}

	if _, err = NewRepository(db).FindByID(ctx, employee); !errors.Is(err, ErrNotFound) {
		t.Fatal("employee UUID still accepted as reference", err)
	}
	if _, err = photos.NewRepository(db).FindByID(ctx, photo); !errors.Is(err, photos.ErrNotFound) {
		t.Fatal("photo UUID still accepted as reference", err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO sessions(role,user_id,employee_id,refresh_hash,expires_at) VALUES('admin','USER-20190102-000001','EMP-20190102-000002','mixed-owner',now()+interval '1 day')`); err == nil {
		t.Fatal("mixed admin/employee session owner accepted")
	}
	e, err := NewRepository(db).FindByID(ctx, "EMP-20190102-000002")
	if err != nil || e.ID != employee || e.MitraID != "MTR-20190102-000001" {
		t.Fatalf("public employee lookup: %+v %v", e, err)
	}
	ph, err := photos.NewRepository(db).FindByID(ctx, "PHO-20190102-000001")
	if err != nil || ph.ID != photo {
		t.Fatalf("photo lookup: %+v %v", ph, err)
	}
	wr, err := workreports.NewRepository(db).FindByAttendanceSession(ctx, "ATT-20190102-000001", 1)
	if err != nil || wr.WorkReportID != "WRP-20190102-000001" {
		t.Fatalf("report lookup: %+v %v", wr, err)
	}
	if _, err = attendances.NewRepository(db).LatestChange(ctx, "2019-01-02"); err != nil {
		t.Fatal("version query still uses UUID join", err)
	}
	a, err := attendances.NewRepository(db).FindByEmployeeDate(ctx, "EMP-20190102-000002", "2019-01-02")
	if err != nil || a.AttendanceID != "ATT-20190102-000001" {
		t.Fatalf("attendance lookup: %+v %v", a, err)
	}
	a, err = attendances.NewRepository(db).Create(ctx, attendances.Attendance{ID: shared.NewID(), EmployeeID: "EMP-20190102-000002", WorkDate: "2019-01-03", CheckInAt: time.Now()})
	if err != nil || !shared.IsPublicID(a.AttendanceID, "ATT") {
		t.Fatalf("attendance insert: %+v %v", a, err)
	}
	wr, created, err := workreports.NewRepository(db).Create(ctx, workreports.WorkReport{ID: shared.NewID(), AttendanceID: a.AttendanceID, SessionNo: 1, PhotoID: "PHO-20190102-000001"})
	if err != nil || !created || !shared.IsPublicID(wr.WorkReportID, "WRP") {
		t.Fatalf("report insert: %+v %v", wr, err)
	}
	duplicate, created, err := workreports.NewRepository(db).Create(ctx, workreports.WorkReport{ID: shared.NewID(), AttendanceID: a.AttendanceID, SessionNo: 1, PhotoID: "PHO-20190102-000001"})
	if err != nil || created || duplicate.WorkReportID != wr.WorkReportID {
		t.Fatalf("duplicate report: %+v %v", duplicate, err)
	}
	// Concurrent workers must receive 1..32 exactly once for a new date.
	var wg sync.WaitGroup
	codes := make(chan string, 32)
	errs := make(chan error, 32)
	for n := 0; n < 32; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			var code string
			err := db.QueryRow(ctx, `INSERT INTO employees(name,phone_hash,created_at) VALUES($1,$1,'2020-02-02T00:00:00Z') RETURNING employee_id`, fmt.Sprintf("parallel-%d", n)).Scan(&code)
			if err != nil {
				errs <- err
			} else {
				codes <- code
			}
		}(n)
	}
	wg.Wait()
	close(codes)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for code := range codes {
		if seen[code] {
			t.Fatalf("duplicate %s", code)
		}
		seen[code] = true
	}
	for n := 1; n <= 32; n++ {
		if !seen[fmt.Sprintf("EMP-20200202-%06d", n)] {
			t.Fatalf("missing sequence %d", n)
		}
	}
	// Updates/reimports keep IDs; deleted numbers cannot be reused; a new day starts at 1.
	var got string
	if err = db.QueryRow(ctx, `INSERT INTO employees(name,phone_hash,created_at) VALUES('parallel-0','parallel-0','2020-02-02T00:00:00Z') ON CONFLICT(phone_hash) DO UPDATE SET name=EXCLUDED.name RETURNING employee_id`).Scan(&got); err != nil || !seen[got] {
		t.Fatalf("upsert ID changed: %s %v", got, err)
	}
	if _, err = db.Exec(ctx, `DELETE FROM employees WHERE employee_id='EMP-20200202-000032'`); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO employees(name,phone_hash,created_at) VALUES('after delete','after-delete','2020-02-02T00:00:00Z') RETURNING employee_id`).Scan(&got); err != nil || got != "EMP-20200202-000033" {
		t.Fatalf("sequence reuse/gap: %s %v", got, err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO employees(name,phone_hash,created_at) VALUES('next day','next-day','2020-02-03T00:00:00Z') RETURNING employee_id`).Scan(&got); err != nil || got != "EMP-20200203-000001" {
		t.Fatalf("daily reset: %s %v", got, err)
	}
	migrate("000010_instansi_to_mitra.down.sql")
	migrate("000009_remove_employee_daerah.down.sql")
	migrate("000008_bidang.down.sql")
	migrate("000006_business_key_references.down.sql")
	migrate("000004_public_ids.down.sql")
	var preserved string
	if err = db.QueryRow(ctx, `SELECT employee_id::text FROM attendances WHERE id=$1`, attendance).Scan(&preserved); err != nil || preserved != employee {
		t.Fatalf("rollback changed FK: %s %v", preserved, err)
	}
	migrate("000004_public_ids.up.sql")
	migrate("000006_business_key_references.up.sql")
	migrate("000008_bidang.up.sql")
	migrate("000009_remove_employee_daerah.up.sql")
	migrate("000010_instansi_to_mitra.up.sql")
}
