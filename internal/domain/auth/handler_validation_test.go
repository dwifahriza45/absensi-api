package auth

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
	"github.com/haidarrasyad/absensi-api/internal/pkg/validation"
)

func TestLoginFieldValidationBeforeAuthentication(t *testing.T) {
	h := NewHandler(nil, false) // invalid requests must never reach credentials/DB.
	cases := []struct {
		name, body string
		employee   bool
		fields     map[string]string
	}{
		{"empty admin", `{"email":"","password":""}`, false, map[string]string{"email": "Email Wajib Diisi", "password": "Password Wajib Diisi"}},
		{"missing admin", `{}`, false, map[string]string{"email": "Email Wajib Diisi", "password": "Password Wajib Diisi"}},
		{"null admin", `null`, false, map[string]string{"email": "Email Wajib Diisi", "password": "Password Wajib Diisi"}},
		{"whitespace admin", `{"email":"  ","password":" \t "}`, false, map[string]string{"email": "Email Wajib Diisi", "password": "Password Wajib Diisi"}},
		{"invalid email", `{"email":"invalid","password":"secret"}`, false, map[string]string{"email": "Format Email Tidak Valid"}},
		{"empty employee", `{"fullname":"","nik":""}`, true, map[string]string{"fullname": "Nama Lengkap Wajib Diisi", "nik": "NIK Wajib Diisi"}},
		{"invalid NIK", `{"fullname":"Employee","nik":"abc"}`, true, map[string]string{"nik": "NIK Harus 16 Digit Angka"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/auth/login", strings.NewReader(tc.body))
			if tc.employee {
				h.LoginEmployee(w, r)
			} else {
				h.LoginAdmin(w, r)
			}
			var got httpx.Response
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if w.Code != 400 || got.Code != 400 || got.Status != "NOK" || !got.Error || got.Message != "Validation Failed" {
				t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
			}
			if len(got.Valid) != len(tc.fields) {
				t.Fatalf("fields: %+v", got.Valid)
			}
			for key, want := range tc.fields {
				if got.Valid[key] != want {
					t.Fatalf("%s: got %q want %q", key, got.Valid[key], want)
				}
			}
			if strings.Contains(w.Body.String(), "secret") {
				t.Fatal("password leaked")
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("invalid input issued auth cookies")
			}
		})
	}
}
func TestIdentifierNormalizationPreservesPassword(t *testing.T) {
	req := AdminLoginRequest{Email: "  admin@example.com  ", Password: " secret with spaces "}
	if err := validation.Struct(&req); err != nil {
		t.Fatal(err)
	}
	if req.Email != "admin@example.com" || req.Password != " secret with spaces " {
		t.Fatalf("unexpected normalization: email=%q", req.Email)
	}
	req.Password = strings.Repeat("é", 37)
	if err := validation.Struct(&req); err == nil {
		t.Fatal("bcrypt byte limit not validated")
	}
	employee := EmployeeLoginRequest{Fullname: "  Example   Employee ", NIK: " 0123456789012345 "}
	if err := validation.Struct(&employee); err != nil {
		t.Fatal(err)
	}
	if employee.Fullname != "Example Employee" || employee.NIK != "0123456789012345" {
		t.Fatal("identifier normalization failed")
	}
}
