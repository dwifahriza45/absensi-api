package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/mitra"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/xuri/excelize/v2"
)

func TestMitraExcelImportExport(t *testing.T) {
	db, _ := concurrencyDB(t)
	ctx := context.Background()
	repo := mitra.NewRepository(db)
	original, err := repo.Save(ctx, "", "IT")
	if err != nil {
		t.Fatal(err)
	}
	tokens := token.NewManager("excel-test-secret", db)
	var user string
	if err = db.QueryRow(ctx, `SELECT user_id FROM users LIMIT 1`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	access, _, _, err := tokens.Issue(ctx, user, token.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	registerMitraRoutes(newRouteGroups(mux), mitra.NewHandler(mitra.NewService(repo)), middleware.NewGuards(tokens))
	call := func(method, path, contentType string, data []byte, auth bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/v1/admin/mitra/"+path, bytes.NewReader(data))
		r.Header.Set("Content-Type", contentType)
		if auth {
			r.AddCookie(&http.Cookie{Name: "access_token", Value: access})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	upload := func(data []byte, auth bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "mitra.xlsx")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(data); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		return call("POST", "import", writer.FormDataContentType(), body.Bytes(), auth, want)
	}
	for _, path := range []string{"export", "import/template"} {
		call("GET", path, "", nil, false, 401)
	}
	template := call("GET", "import/template", "", nil, true, 200)
	if template.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatal("template not Excel")
	}
	workbook := excelize.NewFile()
	defer workbook.Close()
	workbook.SetSheetName("Sheet1", "Mitra")
	values := [][]string{{"mitra_id", "name"}, {original.MitraID, "IT"}, {"", "Finance"}, {"", "Finance"}, {"", ""}, {"MTR-20000101-999999", "Missing"}, {original.MitraID, "IT Updated"}, {original.MitraID, "Finance"}, {original.MitraID, "IT Updated"}, {"", ""}}
	for i, row := range values {
		for j, value := range row {
			axis, _ := excelize.CoordinatesToCellName(j+1, i+1)
			if err = workbook.SetCellStr("Mitra", axis, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	workbook.SetCellFormula("Mitra", "B10", "1+1")
	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	upload(buffer.Bytes(), false, 401)
	result := upload(buffer.Bytes(), true, 200)
	var envelope struct {
		Data mitra.ImportResult `json:"data"`
	}
	if err = json.Unmarshal(result.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	stats := envelope.Data
	if stats.Total != 9 || stats.Success != 2 || stats.Skip != 3 || stats.Error != 4 {
		t.Fatalf("wrong row outcomes: %+v", stats)
	}
	if len(stats.Rows) != 9 || stats.Rows[0].Row != 2 || stats.Rows[8].Row != 10 {
		t.Fatal("wrong Excel row numbers")
	}
	items, err := repo.List(ctx, "")
	if err != nil || len(items) != 2 {
		t.Fatalf("partial import: %+v %v", items, err)
	}
	current, err := repo.Get(ctx, original.MitraID)
	if err != nil || current.Name != "IT Updated" {
		t.Fatalf("savepoint failed: %+v %v", current, err)
	}
	exported := call("GET", "export", "", nil, true, 200)
	if exported.Header().Get("Content-Disposition") == "" {
		t.Fatal("missing attachment header")
	}
	roundtrip := upload(exported.Body.Bytes(), true, 200)
	if err = json.Unmarshal(roundtrip.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Success != 0 || envelope.Data.Skip != 2 || envelope.Data.Error != 0 {
		t.Fatalf("unchanged reimport: %+v", envelope.Data)
	}
	upload([]byte("invalid workbook"), true, 400)
	// JSON remains available for clients that already parse spreadsheets themselves.
	body := []byte(fmt.Sprintf(`{"rows":[{"mitra_id":%q,"name":"IT Updated"},{"name":"JSON New"},{"name":""}]}`, original.MitraID))
	response := call("POST", "import", "application/json", body, true, 200)
	if err = json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Success != 1 || envelope.Data.Skip != 1 || envelope.Data.Error != 1 {
		t.Fatalf("JSON import: %+v", envelope.Data)
	}
	// The user-facing template imports names without any technical ID column.
	simple := excelize.NewFile()
	defer simple.Close()
	simple.SetSheetName("Sheet1", "Mitra")
	for cell, value := range map[string]string{"A1": "NAMA MITRA", "A2": "New Institution", "A4": "New Institution"} {
		if err = simple.SetCellStr("Mitra", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	simpleBuffer, err := simple.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	response = upload(simpleBuffer.Bytes(), true, 200)
	if err = json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Success != 1 || envelope.Data.Skip != 1 || envelope.Data.Error != 1 {
		t.Fatalf("name-only import: %+v", envelope.Data)
	}

}
