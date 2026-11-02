package par

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestMapKeepsOrder(t *testing.T) {
	got := Map(50, 8, func(i int) int {
		time.Sleep(time.Duration(50-i) * 100 * time.Microsecond) // finish in reverse order
		return i * i
	})
	for i, v := range got {
		if v != i*i {
			t.Fatalf("result %d = %d", i, v)
		}
	}
}

func TestMapRespectsLimit(t *testing.T) {
	var running, peak atomic.Int32
	Map(20, 3, func(int) struct{} {
		n := running.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(2 * time.Millisecond)
		running.Add(-1)
		return struct{}{}
	})
	if peak.Load() > 3 || peak.Load() < 2 {
		t.Errorf("peak concurrency %d, want 2..3", peak.Load())
	}
}

func TestMapEdgeCases(t *testing.T) {
	if got := Map(0, 4, func(int) int { return 1 }); len(got) != 0 {
		t.Error("n=0 must return nothing")
	}
	if got := Map(3, 0, func(i int) int { return i }); len(got) != 3 {
		t.Error("a limit below 1 must still make progress")
	}
}
