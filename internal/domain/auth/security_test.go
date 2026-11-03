package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/domain/employees"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

type employeeStub struct{ employee employees.Employee }

func (e employeeStub) GetByID(context.Context, string) (employees.Employee, error) {
	return e.employee, nil
}
func (e employeeStub) GetByNIK(context.Context, string) (employees.Employee, error) {
	return e.employee, nil
}
func TestEmployeeLoginRequiresRegisteredNIK(t *testing.T) {
	hasher := shared.NewPhoneHasher("test-phone-secret")
	s := NewService(employeeStub{employees.Employee{ID: shared.NewID(), Name: "Employee", NIK: "0123456789012345", IsActive: true}}, nil, nil, hasher, 3000)
	for _, nik := range []string{"", "0123456789012346", "not-a-number", "081234567890"} {
		if _, err := s.LoginEmployee(context.Background(), "test", EmployeeLoginRequest{Fullname: "Employee", NIK: nik}); err != ErrInvalidCredentials {
			t.Fatalf("nik %q: %v", nik, err)
		}
	}
}
func TestSessionTokensOnlyInHTTPOnlyCookies(t *testing.T) {
	session := SessionResponse{Token: "secret-access", RefreshToken: "secret-refresh", ExpiresAt: time.Now().Add(time.Minute)}
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-") {
		t.Fatal("token exposed in JSON")
	}
	h := NewHandler(nil, true)
	w := httptest.NewRecorder()
	h.cookies(w, session, token.RoleAdmin)
	cookies := w.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatal("missing cookies")
	}
	for _, c := range cookies {
		if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/api/v1/admin" {
			t.Fatalf("unsafe cookie %+v", c)
		}
	}
}

func TestSharedOfficeIPDoesNotUseAdminLoginBudget(t *testing.T) {
	hasher := shared.NewPhoneHasher("test-secret")
	s := NewService(employeeStub{employees.Employee{ID: shared.NewID(), Name: "Employee", NIK: "0123456789012345", IsActive: true}}, nil, nil, hasher, 3000)
	for i := 0; i < 1500; i++ {
		_, err := s.LoginEmployee(context.Background(), "office-ip", EmployeeLoginRequest{Fullname: fmt.Sprintf("Employee %d", i), NIK: "0123456789012346"})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("employee %d incorrectly blocked: %v", i, err)
		}
	}
	if !s.adminRequests.Allow("login:office-ip") {
		t.Fatal("employees exhausted admin budget")
	}
	for i := 0; i < 5; i++ {
		_, _ = s.LoginEmployee(context.Background(), "office-ip", EmployeeLoginRequest{Fullname: "Same Employee", NIK: "0123456789012346"})
	}
	if _, err := s.LoginEmployee(context.Background(), "office-ip", EmployeeLoginRequest{Fullname: "Same Employee", NIK: "0123456789012346"}); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatal("per-account brute force limit disabled")
	}
}
