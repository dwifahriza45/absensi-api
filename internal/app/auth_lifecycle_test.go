package app

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/app/middleware"
	"github.com/haidarrasyad/absensi-api/internal/domain/auth"
	"github.com/haidarrasyad/absensi-api/internal/pkg/token"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

// Exercise actual HTTP cookies, routes and DB sessions, rather than token methods alone.
func TestAdminCookieLifecycle(t *testing.T) {
	db, _ := concurrencyDB(t)
	manager := token.NewManager("isolated-http-test-secret", db)
	svc := auth.NewService(nil, auth.NewRepository(db), manager, shared.NewPhoneHasher("test"), 3000)
	mux := http.NewServeMux()
	registerAuthRoutes(newRouteGroups(mux), auth.NewHandler(svc, false), middleware.NewGuards(manager))
	server := httptest.NewServer(mux)
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	base := server.URL + "/api/v1/admin/auth/"
	request := func(method, path, body string, want int) {
		t.Helper()
		req, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("%s: status %d, want %d: %s", path, res.StatusCode, want, b)
		}
	}
	for range 5 {
		request("POST", "login", `{"email":"admin@absensi.local","password":"Admin123!"}`, 200)
		request("GET", "me", "", 200)
	}
	req, _ := http.NewRequest("GET", base+"me", nil)
	oldCookies := jar.Cookies(req.URL)
	request("POST", "refresh", "", 200)
	request("GET", "me", "", 200)
	// Replaying an already rotated refresh token must fail.
	stale := &http.Client{Timeout: 15 * time.Second}
	replay, _ := http.NewRequest("POST", base+"refresh", nil)
	for _, c := range oldCookies {
		replay.AddCookie(c)
	}
	res, err := stale.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("refresh replay: %d", res.StatusCode)
	}
	// Failed replay must not destroy the legitimate, rotated session.
	request("GET", "me", "", 200)
	liveCookies := jar.Cookies(req.URL)
	request("POST", "logout", "", 200)
	request("GET", "me", "", 401)
	request("POST", "refresh", "", 401)
	// Logout revokes the DB session, not just cookies on this client.
	for _, c := range liveCookies {
		if c.Name == "access_token" {
			if _, err := manager.Parse(context.Background(), c.Value); err == nil {
				t.Fatal("logged-out access token remains valid")
			}
		}
		if c.Name == "refresh_token" {
			if _, _, _, err := manager.Refresh(context.Background(), c.Value, token.RoleAdmin); err == nil {
				t.Fatal("logged-out refresh token remains valid")
			}
		}
	}
	request("POST", "logout", "", 200)
}
