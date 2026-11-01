package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/jackc/pgx/v5"
)

type refreshSessions struct{ logoutSessions }

func (s refreshSessions) Refresh(context.Context, string, token.Role) (string, string, time.Time, error) {
	return "", "", time.Time{}, s.err
}
func TestRefreshErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cookie bool
		err    error
		want   int
	}{
		{"missing cookie", false, nil, 401},
		{"expired or rotated", true, pgx.ErrNoRows, 401},
		{"database failure", true, errors.New("database unavailable"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler(&Service{tokens: refreshSessions{logoutSessions{tc.err}}}, false)
			req := httptest.NewRequest("POST", "/api/v1/admin/auth/refresh", nil)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "test"})
			}
			w := httptest.NewRecorder()
			h.Refresh(w, req, token.RoleAdmin)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("failed refresh changed cookies")
			}
		})
	}
}
