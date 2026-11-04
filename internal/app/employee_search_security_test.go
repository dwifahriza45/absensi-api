package app

import (
	"context"
	"encoding/json"
	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestEmployeeSearchLimitAndLiteralSQLInput(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `INSERT INTO employees(name,phone_hash) SELECT 'Search Employee '||n, 'search-phone-'||n FROM generate_series(1,15) n`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO employees(name,phone_hash,is_active) VALUES('Search Hidden Employee','hidden-phone',false)`); err != nil {
		t.Fatal(err)
	}
	handler := employees.NewHandler(employees.NewService(employees.NewRepository(db)))
	for _, tc := range []struct {
		query string
		count int
	}{{"Search", 10}, {"' OR 1=1 --", 0}, {"Search%", 0}, {"Search_", 0}, {"'; DROP TABLE employees; --", 0}} {
		w := httptest.NewRecorder()
		handler.Search(w, httptest.NewRequest(http.MethodGet, "/api/v1/karyawan/employees/search?q="+url.QueryEscape(tc.query)+"&limit=10000", nil))
		if w.Code != 200 {
			t.Fatalf("query %q returned %d", tc.query, w.Code)
		}
		var result struct {
			Data []map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Data) != tc.count {
			t.Fatalf("query %q returned %d results", tc.query, len(result.Data))
		}
		for _, item := range result.Data {
			if _, exists := item["phone"]; exists {
				t.Fatal("search exposed phone")
			}
		}
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM employees`).Scan(&count); err != nil || count != 16 {
		t.Fatalf("search mutated employees: %v", err)
	}
	rows, err := employees.NewRepository(db).Search(ctx, "Search", 10000)
	if err != nil || len(rows) != 10 {
		t.Fatalf("repository limit bypassed: %d %v", len(rows), err)
	}
}
