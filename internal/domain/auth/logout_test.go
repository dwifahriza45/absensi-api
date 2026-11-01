package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/pkg/httpx"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
)

type logoutSessions struct{ err error }

func (logoutSessions) Issue(context.Context, string, token.Role) (string, string, time.Time, error) {
	panic("unexpected Issue")
}
func (logoutSessions) Refresh(context.Context, string, token.Role) (string, string, time.Time, error) {
	panic("unexpected Refresh")
}
func (s logoutSessions) Revoke(context.Context, string, token.Role, ...string) error { return s.err }
func TestLogoutTimeoutDoesNotReportSuccessOrClearCookies(t *testing.T) {
	for _, err := range []error{token.ErrRevocationTimeout, errors.Join(errors.New("DB timeout"), token.ErrRevocationTimeout)} {
		h := NewHandler(&Service{tokens: logoutSessions{err}}, false)
		w := httptest.NewRecorder()
		h.Logout(w, httptest.NewRequest("POST", "/api/v1/admin/auth/logout", nil), token.RoleAdmin)
		var body httpx.Response
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != 504 || body.Code != 504 || body.Status != "NOK" || !body.Error {
			t.Fatalf("timeout response: %d %s", w.Code, w.Body.String())
		}
		if len(w.Result().Cookies()) != 0 {
			t.Fatal("cleared cookies before confirming session revocation")
		}
		if w.Header().Get("Retry-After") != "2" {
			t.Fatal("missing retry hint")
		}
	}
}
func TestSuccessfulLogoutStillClearsCookies(t *testing.T) {
	h := NewHandler(&Service{tokens: logoutSessions{}}, false)
	w := httptest.NewRecorder()
	h.Logout(w, httptest.NewRequest("POST", "/api/v1/admin/auth/logout", nil), token.RoleAdmin)
	if w.Code != 200 || len(w.Result().Cookies()) != 2 {
		t.Fatalf("logout: %d %s", w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.MaxAge != -1 || c.Path != "/api/v1/admin" {
			t.Fatalf("cookie not cleared: %s", c.Name)
		}
	}
}
