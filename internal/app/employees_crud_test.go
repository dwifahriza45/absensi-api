package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

func TestEmployeeCRUDAdmin(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	manager := token.NewManager("crud-integration-secret", db)
	var adminID, mitraID, bidangID string
	if err := db.QueryRow(ctx, `SELECT user_id FROM users LIMIT 1`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO mitra(name) VALUES('CRUD Institution') RETURNING mitra_id`).Scan(&mitraID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO bidang(name) VALUES('CRUD Division') RETURNING bidang_id`).Scan(&bidangID); err != nil {
		t.Fatal(err)
	}
	access, _, _, err := manager.Issue(ctx, adminID, token.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerEmployeesRoutes(newRouteGroups(mux), employees.NewHandler(employees.NewService(employees.NewRepository(db))), middleware.NewGuards(manager))
	call := func(method, path, body string, authenticated bool, want int) json.RawMessage {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1/admin/employees"+path, strings.NewReader(body))
		if authenticated {
			req.AddCookie(&http.Cookie{Name: "access_token", Value: access})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Data
	}
	body := `{"nik":"0123456789012345","name":"CRUD Employee","mitra_id":"` + mitraID + `","bidang_id":"` + bidangID + `"}`
	for _, method := range []string{"POST", "GET", "PUT", "DELETE"} {
		path := ""
		if method != "POST" {
			path = "/EMP-20261031-000001"
		}
		call(method, path, body, false, 401)
	}
	call("POST", "", `{}`, true, 400)
	missing := strings.ReplaceAll(body, mitraID, "MTR-20000101-999999")
	call("POST", "", missing, true, 400)
	var e employees.Response
	if err := json.Unmarshal(call("POST", "", body, true, 201), &e); err != nil {
		t.Fatal(err)
	}
	if !shared.IsPublicID(e.EmployeeID, "EMP") || e.MitraID != mitraID || !e.IsActive {
		t.Fatalf("invalid created employee: %+v", e)
	}
	call("POST", "", body, true, 409)
	call("GET", "/"+e.EmployeeID, "", true, 200)
	call("GET", "/"+e.ID, "", true, 404)
	employeeAccess, refresh, _, err := manager.Issue(ctx, e.EmployeeID, token.RoleEmployee)
	if err != nil {
		t.Fatal(err)
	}
	update := `{"nik":"0123456789012346","name":"Edited Employee","mitra_id":"` + mitraID + `","bidang_id":"` + bidangID + `"}`
	call("PUT", "/"+e.EmployeeID, update, true, 200)
	if _, err := manager.Parse(ctx, employeeAccess); err == nil {
		t.Fatal("profile edit did not revoke login")
	}
	if _, _, _, err := manager.Refresh(ctx, refresh, token.RoleEmployee); err == nil {
		t.Fatal("profile edit did not revoke refresh")
	}
	// Ensure deleting a profile preserves its attendance history and business ID.
	if _, err := db.Exec(ctx, `INSERT INTO attendances(id,employee_id,work_date,check_in_at) VALUES(gen_random_uuid(),$1,'2026-10-31',now())`, e.EmployeeID); err != nil {
		t.Fatal(err)
	}
	employeeAccess, _, _, err = manager.Issue(ctx, e.EmployeeID, token.RoleEmployee)
	if err != nil {
		t.Fatal(err)
	}
	call("DELETE", "/"+e.EmployeeID, "", true, 409)
	call("PUT", "/"+e.EmployeeID, `{"nik":"0123456789012346","name":"Inactive Employee","mitra_id":"`+mitraID+`","bidang_id":"`+bidangID+`","isActive":false}`, true, 200)
	if _, err := manager.Parse(ctx, employeeAccess); err == nil {
		t.Fatal("deleted employee session remains valid")
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM attendances WHERE employee_id=$1`, e.EmployeeID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("history missing: %d %v", count, err)
	}
	var list []employees.Response
	if err := json.Unmarshal(call("GET", "", "", true, 200), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatal("deleted employee in active list")
	}
	if err := json.Unmarshal(call("GET", "?includeInactive=true", "", true, 200), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].IsActive {
		t.Fatal("inactive employee not listed")
	}
	call("PUT", "/"+e.EmployeeID, `{"nik":"0123456789012346","name":"Restored Employee","mitra_id":"`+mitraID+`","bidang_id":"`+bidangID+`","isActive":true}`, true, 200)
	call("GET", "/"+e.EmployeeID, "", true, 200)
	call("DELETE", "/EMP-20000101-999999", "", true, 404)
}
