package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRejectTrailingJSON(t *testing.T) {
	for _, body := range []string{`{"name":"a"}{"name":"b"}`, `{"name":"a"}garbage`, "{\"name\":\"a\"}" + strings.Repeat(" ", 100) + "x"} {
		var req struct {
			Name string `json:"name"`
		}
		if err := DecodeJSONLimit(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(body)), &req, 60); err == nil {
			t.Fatalf("accepted body %q", body)
		}
	}
}
func TestRejectNonFiniteCoordinates(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf"} {
		r := httptest.NewRequest("POST", "/", strings.NewReader("lat="+value))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if _, err := OptionalFloat(r, "lat"); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
}
