package token

import (
	"context"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run against a disposable PostgreSQL database with TEST_DATABASE_URL.
func TestSessionLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for database session integration tests")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	schema := "test_sessions_" + time.Now().Format("150405000000")
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(ctx, `CREATE TABLE users(id uuid PRIMARY KEY,user_id text UNIQUE,active boolean,role text);
 CREATE TABLE employees(id uuid PRIMARY KEY,employee_id text UNIQUE,is_active boolean);
 CREATE SEQUENCE test_session_sequence;
 CREATE TABLE sessions(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),session_id text UNIQUE DEFAULT ('SES-20261031-'||lpad(nextval('test_session_sequence')::text,6,'0')),user_id text,employee_id text,role text,refresh_hash text UNIQUE,expires_at timestamptz);
 INSERT INTO users VALUES('00000000-0000-0000-0000-000000000001','USER-20261031-000001',true,'admin');
 INSERT INTO employees VALUES('00000000-0000-0000-0000-000000000002','EMP-20261031-000001',true);`)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager("test-secret-longer-than-thirty-two-characters", db)
	for _, tc := range []struct {
		role   Role
		id     string
		table  string
		active string
	}{
		{RoleAdmin, "USER-20261031-000001", "users", "active"},
		{RoleEmployee, "EMP-20261031-000001", "employees", "is_active"},
	} {
		t.Run(string(tc.role), func(t *testing.T) {
			access, refresh, _, err := m.Issue(ctx, tc.id, tc.role)
			if err != nil {
				t.Fatal(err)
			}
			c, err := m.Parse(ctx, access)
			if err != nil || c.Subject != tc.id || c.Role != tc.role {
				t.Fatalf("claims %v: %v", c, err)
			}
			other := RoleAdmin
			if tc.role == RoleAdmin {
				other = RoleEmployee
			}
			if _, _, _, err = m.Refresh(ctx, refresh, other); err == nil {
				t.Fatal("cross-role refresh accepted")
			}
			nextAccess, nextRefresh, _, err := m.Refresh(ctx, refresh, tc.role)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, _, err = m.Refresh(ctx, refresh, tc.role); err == nil {
				t.Fatal("refresh replay accepted")
			}
			if _, err = m.Parse(ctx, nextAccess); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, "UPDATE "+tc.table+" SET "+tc.active+"=false"); err != nil {
				t.Fatal(err)
			}
			if _, err = m.Parse(ctx, nextAccess); err == nil {
				t.Fatal("inactive account accepted")
			}
			if _, _, _, err = m.Refresh(ctx, nextRefresh, tc.role); err == nil {
				t.Fatal("inactive account refreshed")
			}
			if _, err = db.Exec(ctx, "UPDATE "+tc.table+" SET "+tc.active+"=true"); err != nil {
				t.Fatal(err)
			}
			if err = m.Revoke(ctx, nextRefresh, tc.role); err != nil {
				t.Fatal(err)
			}
			if _, err = m.Parse(ctx, access); err == nil {
				t.Fatal("revoked access accepted")
			}
			if _, err = m.Parse(ctx, nextAccess); err == nil {
				t.Fatal("revoked rotated access accepted")
			}
		})
	}
	t.Run("parallel refresh and logout", func(t *testing.T) {
		const subject = "USER-20261031-000001"
		access, refresh, _, err := m.Issue(ctx, subject, RoleAdmin)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		results := make(chan string, 24)
		failures := make(chan error, 24)
		for n := 0; n < 24; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				a, _, _, err := m.Refresh(ctx, refresh, RoleAdmin)
				if err != nil {
					failures <- err
				} else {
					results <- a
				}
			}()
		}
		wg.Wait()
		close(results)
		close(failures)
		for err := range failures {
			if !errors.Is(err, pgx.ErrNoRows) {
				t.Fatal(err)
			}
		}
		winner := ""
		count := 0
		for a := range results {
			winner = a
			count++
		}
		if count != 1 {
			t.Fatalf("refresh winners: %d", count)
		}
		// Logout using pre-rotation cookies must still revoke the session.
		if err = m.Revoke(ctx, refresh, RoleAdmin, access); err != nil {
			t.Fatal(err)
		}
		if _, err = m.Parse(ctx, winner); err == nil {
			t.Fatal("rotated session survived logout")
		}
		for n := 0; n < 12; n++ {
			a, r, _, err := m.Issue(ctx, subject, RoleAdmin)
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			done := make(chan error, 2)
			newAccess := make(chan string, 1)
			go func() { <-start; next, _, _, err := m.Refresh(ctx, r, RoleAdmin); newAccess <- next; done <- err }()
			go func() { <-start; done <- m.Revoke(ctx, r, RoleAdmin, a) }()
			close(start)
			for j := 0; j < 2; j++ {
				if err := <-done; err != nil && !errors.Is(err, pgx.ErrNoRows) {
					t.Fatal(err)
				}
			}
			if next := <-newAccess; next != "" {
				if _, err = m.Parse(ctx, next); err == nil {
					t.Fatal("concurrent refresh escaped logout")
				}
			}
		}
		a, r, _, err := m.Issue(ctx, subject, RoleAdmin)
		if err != nil {
			t.Fatal(err)
		}
		claims, err := m.Parse(ctx, a)
		if err != nil {
			t.Fatal(err)
		}
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
		expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
		if err != nil {
			t.Fatal(err)
		}
		next, _, _, err := m.Refresh(ctx, r, RoleAdmin)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = m.Parse(ctx, expired); err == nil {
			t.Fatal("expired access authorized request")
		}
		if err = m.Revoke(ctx, r, RoleAdmin, expired); err != nil {
			t.Fatal(err)
		}
		if _, err = m.Parse(ctx, next); err == nil {
			t.Fatal("logout with expired signed access failed")
		}
	})

	t.Run("revocation respects deadline while row locked", func(t *testing.T) {
		access, refresh, _, err := m.Issue(ctx, "USER-20261031-000001", RoleAdmin)
		if err != nil {
			t.Fatal(err)
		}
		blocker, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer blocker.Rollback(ctx)
		if _, err = blocker.Exec(ctx, `SELECT session_id FROM sessions WHERE refresh_hash=$1 FOR UPDATE`, hash(refresh)); err != nil {
			t.Fatal(err)
		}
		short, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		start := time.Now()
		err = m.Revoke(short, refresh, RoleAdmin, access)
		if !errors.Is(err, ErrRevocationTimeout) {
			t.Fatalf("expected controlled timeout: %v", err)
		}
		if elapsed := time.Since(start); elapsed < 9*time.Second || elapsed > 11500*time.Millisecond {
			t.Fatalf("default 10-second timeout not enforced: %s", elapsed)
		}
		// The DELETE cannot commit while another transaction holds the row lock.
		if _, err = m.Parse(ctx, access); err != nil {
			t.Fatal("blocked session disappeared", err)
		}
		if err = blocker.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if err = m.Revoke(ctx, refresh, RoleAdmin, access); err != nil {
			t.Fatal(err)
		}
		if _, err = m.Parse(ctx, access); err == nil {
			t.Fatal("session survived successful retry")
		}
	})

}
