package shared

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterConcurrentAttempts(t *testing.T) {
	l := NewAttemptLimiter(5, time.Minute)
	var wg sync.WaitGroup
	var allowed atomic.Int32
	for n := 0; n < 100; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("ip") {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 5 {
		t.Fatalf("allowed %d attempts", allowed.Load())
	}
	l.Reset("ip")
	if !l.Allow("ip") {
		t.Fatal("reset failed")
	}
}
