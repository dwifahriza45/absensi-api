package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingBody struct{ reads atomic.Int32 }

func (b *countingBody) Read([]byte) (int, error) { b.reads.Add(1); return 0, io.EOF }
func TestUploadAdmissionRejectsBeforeReadingBody(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	handler := UploadAdmission(1, 20*time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(entered) })
		<-release
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(200)
	}))
	go func() {
		defer close(done)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader("first")))
	}()
	<-entered
	body := &countingBody{}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/", body))
	if w.Code != 503 || w.Header().Get("Retry-After") != "2" {
		t.Fatalf("backpressure: %d %s", w.Code, w.Body.String())
	}
	if body.reads.Load() != 0 {
		t.Fatal("rejected upload body was read into memory")
	}
	close(release)
	<-done
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader("third")))
	if w.Code != 200 {
		t.Fatal("upload slot leaked")
	}
}
