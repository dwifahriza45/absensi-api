package workreports

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
)

func TestEmptyMultipartReportsAllRequiredFields(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/work-reports", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	NewHandler(nil).Submit(w, r)
	var got httpx.Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != 400 || got.Code != 400 || got.Status != "NOK" || !got.Error || got.Valid["session_no"] != "Sesi Foto Wajib Diisi" || got.Valid["photo"] != "Foto Wajib Diisi" {
		t.Fatalf("response: %d %s", w.Code, w.Body.String())
	}
}
