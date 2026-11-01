package httpx

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
)

func TestCoffeeAPIResponseContract(t *testing.T) {
	for _, status := range []int{200, 201} {
		w := httptest.NewRecorder()
		OK(w, status, map[string]string{"id": "EMP-20261031-000001"}, "Created Successfully")
		var got Response
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if w.Code != status || got.Code != status || got.Status != "OK" || got.Error || got.Data == nil || got.Message != "Created Successfully" {
			t.Fatalf("success: %s", w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "/", nil)
	for _, err := range []error{apperror.Unauthorized("unauthorized", "Sesi tidak valid."), errors.New("PRIVATE DATABASE DETAILS")} {
		w := httptest.NewRecorder()
		Fail(w, r, err)
		var got map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got["code"] != float64(w.Code) || got["status"] != "NOK" || got["error"] != true || got["message"] == "PRIVATE DATABASE DETAILS" {
			t.Fatalf("error: %s", w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	OK(w, 200, nil, "Logout Successfully")
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if _, exists := got["data"]; exists {
		t.Fatal("nil data differs from coffee-api omitempty contract")
	}
	if _, exists := got["valid"]; exists {
		t.Fatal("success unexpectedly contains validation errors")
	}
}
