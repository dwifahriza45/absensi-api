package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/bidang"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

func TestBidangCRUDAndEmployeeReferences(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	tokens := token.NewManager("bidang-integration-secret", db)
	var userID, mitraID, otherBidangID string
	if err := db.QueryRow(ctx, `SELECT user_id FROM users LIMIT 1`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO mitra(name) VALUES('Bidang Office') RETURNING mitra_id`).Scan(&mitraID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO bidang(name) VALUES('Other Division') RETURNING bidang_id`).Scan(&otherBidangID); err != nil {
		t.Fatal(err)
	}
	admin, _, _, err := tokens.Issue(ctx, userID, token.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	groups := newRouteGroups(mux)
	guards := middleware.NewGuards(tokens)
	registerBidangRoutes(groups, bidang.NewHandler(bidang.NewService(bidang.NewRepository(db))), guards)
	registerEmployeesRoutes(groups, employees.NewHandler(employees.NewService(employees.NewRepository(db))), guards)
	call := func(method, path, body string, auth bool, want int) json.RawMessage {
		t.Helper()
		r := httptest.NewRequest(method, "/api/v1/admin/"+path, strings.NewReader(body))
		if auth {
			r.AddCookie(&http.Cookie{Name: "access_token", Value: admin})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
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
	for _, method := range []string{"POST", "GET", "PUT", "DELETE"} {
		path := "bidang"
		if method == "PUT" || method == "DELETE" {
			path += "/BID-20261031-000001"
		}
		call(method, path, `{"name":"IT"}`, false, 401)
	}
	call("POST", "bidang", `{"name":" "}`, true, 400)
	var b bidang.Response
	if err := json.Unmarshal(call("POST", "bidang", `{"name":"IT"}`, true, 201), &b); err != nil {
		t.Fatal(err)
	}
	if !shared.IsPublicID(b.BidangID, "BID") || !shared.IsID(b.ID) {
		t.Fatalf("bad IDs: %+v", b)
	}
	call("POST", "bidang", `{"name":"IT"}`, true, 409)
	call("GET", "bidang/"+b.ID, "", true, 404)
	call("GET", "bidang/"+b.BidangID, "", true, 200)
	employeeBody := `{"nik":"0123456789012345","name":"Bidang Employee","mitra_id":"` + mitraID + `","bidang_id":"` + b.BidangID + `"}`
	call("POST", "employees", strings.ReplaceAll(employeeBody, b.BidangID, "BID-20000101-999999"), true, 400)
	var e employees.Response
	if err := json.Unmarshal(call("POST", "employees", employeeBody, true, 201), &e); err != nil {
		t.Fatal(err)
	}
	if e.BidangID != b.BidangID || e.Bidang != "IT" {
		t.Fatalf("missing reference: %+v", e)
	}
	var usage bidang.Response
	if err := json.Unmarshal(call("GET", "bidang/"+b.BidangID, "", true, 200), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.CanDelete || usage.EmployeeCount != 1 || usage.DeleteReason == "" {
		t.Fatalf("missing relation metadata: %+v", usage)
	}
	call("DELETE", "bidang/"+b.BidangID, "", true, 409)
	call("PUT", "bidang/"+b.BidangID, `{"name":"Information Technology"}`, true, 200)
	if err := json.Unmarshal(call("GET", "employees/"+e.EmployeeID, "", true, 200), &e); err != nil {
		t.Fatal(err)
	}
	if e.Bidang != "Information Technology" {
		t.Fatal("rename not reflected on employee")
	}
	call("GET", "employees?bidang_id="+b.BidangID, "", true, 200)
	call("GET", "bidang?q=Technology", "", true, 200)
	call("PUT", "employees/"+e.EmployeeID, `{"nik":"0123456789012345","name":"Bidang Employee","mitra_id":"`+mitraID+`","bidang_id":"`+b.BidangID+`","isActive":false}`, true, 200)
	usage = bidang.Response{}
	if err := json.Unmarshal(call("GET", "bidang/"+b.BidangID, "", true, 200), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.CanDelete || usage.EmployeeCount != 1 || usage.DeleteReason == "" {
		t.Fatalf("missing relation metadata: %+v", usage)
	}
	call("DELETE", "bidang/"+b.BidangID, "", true, 409)
	// Move to another required division before deleting the previous master.
	call("PUT", "employees/"+e.EmployeeID, `{"nik":"0123456789012345","name":"Bidang Employee","bidang_id":"`+otherBidangID+`","mitra_id":"`+mitraID+`"}`, true, 200)
	call("DELETE", "bidang/"+b.BidangID, "", true, 200)
	call("GET", "bidang/"+b.BidangID, "", true, 404)
	call("PUT", "bidang/BID-20000101-999999", `{"name":"Missing"}`, true, 404)
	// Concurrent creations receive unique business IDs from the atomic counter.
	var wg sync.WaitGroup
	codes := make(chan string, 12)
	failures := make(chan error, 12)
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			item, err := bidang.NewService(bidang.NewRepository(db)).Save(ctx, "", bidang.WriteRequest{Name: fmt.Sprintf("Parallel %d", i)})
			if err != nil {
				failures <- err
				return
			}
			codes <- item.BidangID
		}(i)
	}
	wg.Wait()
	close(codes)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	seen := map[string]bool{}
	for code := range codes {
		if seen[code] {
			t.Fatal("duplicate BID")
		}
		seen[code] = true
	}
	if len(seen) != 12 {
		t.Fatalf("created %d of 12", len(seen))
	}
}
