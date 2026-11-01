package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserMutationProtection(t *testing.T) {
	for _, tc := range []struct {
		origin, site string
		want         int
	}{
		{"https://evil.example", "cross-site", 403}, {"null", "cross-site", 403}, {"", "cross-site", 403},
		{"https://app.example", "same-site", 200}, {"", "", 200},
	} {
		r := httptest.NewRequest("POST", "/", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		w := httptest.NewRecorder()
		Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }), SecurityHeaders, CORS([]string{"https://app.example"})).ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%+v: got %d", tc, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("missing security headers")
		}
	}
}

func TestRecoverKeepsHandlingRequests(t *testing.T) {
	handler := Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/panic" {
			panic("intentional test panic")
		}
		w.WriteHeader(http.StatusOK)
	}))
	for _, tc := range []struct {
		path   string
		status int
	}{{"/panic", 500}, {"/health", 200}} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: got %d", tc.path, w.Code)
		}
	}
}
