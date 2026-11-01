package employees

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

func TestEmployeeCreateUpdateRequireDivision(t *testing.T) {
	for _, body := range []string{
		`{"name":"Employee","phone":"081234567890","mitra_id":"MTR-20261031-000001"}`,
		`{"name":"Employee","phone":"081234567890","mitra_id":"MTR-20261031-000001","bidang_id":""}`,
		`{"name":"Employee","phone":"081234567890","mitra_id":"MTR-20261031-000001","bidang_id":"  "}`,
		`{"name":"Employee","phone":"081234567890","mitra_id":"MTR-20261031-000001","bidang_id":null}`,
	} {
		for _, method := range []string{"POST", "PUT"} {
			h := NewHandler(nil) // Invalid requests must fail before service/DB access.
			r := httptest.NewRequest(method, "/", strings.NewReader(body))
			r.SetPathValue("employee_id", "EMP-20261031-000001")
			w := httptest.NewRecorder()
			if method == "POST" {
				h.Create(w, r)
			} else {
				h.Update(w, r)
			}
			var result httpx.Response
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != 400 || result.Valid["bidang_id"] != "Bidang Wajib Diisi" {
				t.Fatalf("%s: %d %s", method, w.Code, w.Body.String())
			}
		}
	}
}

type importCapture struct {
	Repository
	rows   []Employee
	called bool
}

func (r *importCapture) Import(_ context.Context, rows []Employee, _ bool) (ImportResult, error) {
	r.rows = rows
	r.called = true
	return ImportResult{Created: len(rows)}, nil
}
func TestImportSkipsMissingDivision(t *testing.T) {
	repo := &importCapture{}
	s := NewService(repo, shared.NewPhoneHasher("test-secret"))
	valid := ImportRow{Name: "Valid", Phone: "081234567890", Bidang: "IT"}
	missing := valid
	missing.Name = "Missing Division"
	missing.Bidang = "  "
	result, err := s.Import(context.Background(), ImportRequest{Rows: []ImportRow{missing, valid}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || len(result.Skipped) != 1 || len(repo.rows) != 1 || result.Skipped[0].Reason != "Bidang Wajib Diisi" {
		t.Fatalf("unexpected import: %+v", result)
	}
	repo.called = false
	_, err = s.Import(context.Background(), ImportRequest{Rows: []ImportRow{missing}})
	if !errors.Is(err, ErrImportEmpty) || repo.called {
		t.Fatal("invalid import reached repository")
	}
}
