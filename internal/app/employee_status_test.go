package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
)

func TestEmployeeStatusOnlyChangesStatusAndRevokesSessions(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	id := newEmployee(t, db)
	repo := employees.NewRepository(db)
	tokens := token.NewManager("status-test-secret", db)
	var user string
	if err := db.QueryRow(ctx, `SELECT user_id FROM users LIMIT 1`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	admin, _, _, err := tokens.Issue(ctx, user, token.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	access, refresh, _, err := tokens.Issue(ctx, id, token.RoleEmployee)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerEmployeesRoutes(newRouteGroups(mux), employees.NewHandler(employees.NewService(repo)), middleware.NewGuards(tokens))
	call := func(body string, auth bool, want int) {
		t.Helper()
		r := httptest.NewRequest("PUT", "/api/v1/admin/employees/"+id+"/status", strings.NewReader(body))
		if auth {
			r.AddCookie(&http.Cookie{Name: "access_token", Value: admin})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	call(`{"isActive":false}`, false, 401)
	for _, body := range []string{`{}`, `{"isActive":null}`, `{"isActive":"false"}`, `{"isActive":false,"name":"Override"}`} {
		call(body, true, 400)
	}
	call(`{"isActive":false}`, true, 200)
	e, err := repo.FindByID(ctx, id)
	if err != nil || e.IsActive || e.Name != "Concurrent" || e.BidangID != "" {
		t.Fatalf("profile overwritten: %+v %v", e, err)
	}
	if _, err = tokens.Parse(ctx, access); err == nil {
		t.Fatal("disabled employee session remains valid")
	}
	call(`{"isActive":true}`, true, 200)
	e, err = repo.FindByID(ctx, id)
	if err != nil || !e.IsActive {
		t.Fatal("employee not reactivated")
	}
	if _, _, _, err = tokens.Refresh(ctx, refresh, token.RoleEmployee); err == nil {
		t.Fatal("reactivation restored revoked session")
	}
}
