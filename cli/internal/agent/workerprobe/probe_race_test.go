package workerprobe

import (
	"fmt"
	"io"
	"sync"
	"testing"
)

func TestWorkerProbeTeeJSONLConcurrentWrites(t *testing.T) {
	const goroutines = 8
	const perGoroutine = 64
	const total = goroutines * perGoroutine

	probe := New(Identity{}, Config{BufferCap: total})
	tee := TeeJSONL(io.Discard, probe)

	start := make(chan struct{})
	var wg sync.WaitGroup
	var writeMu sync.Mutex
	var writeErr error

	for g := 0; g < goroutines; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < perGoroutine; i++ {
				line := fmt.Sprintf(`{"type":"loop.tick","data":{"bead_id":"bead-%d","attempt_id":"attempt-%d"}}`, g, i)
				if _, err := tee.Write(append([]byte(line), '\n')); err != nil {
					writeMu.Lock()
					if writeErr == nil {
						writeErr = err
					}
					writeMu.Unlock()
					return
				}
			}
		}()
	}

	close(start)
	wg.Wait()

	writeMu.Lock()
	defer writeMu.Unlock()
	if writeErr != nil {
		t.Fatalf("tee write failed: %v", writeErr)
	}
	if got := len(probe.buf); got != total {
		t.Fatalf("expected %d mirrored events, got %d", total, got)
	}
	for i, ev := range probe.buf {
		if ev.Kind != "loop.tick" {
			t.Fatalf("event %d: expected kind loop.tick, got %q", i, ev.Kind)
		}
		if ev.BeadID == "" {
			t.Fatalf("event %d: missing bead_id", i)
		}
		if ev.AttemptID == "" {
			t.Fatalf("event %d: missing attempt_id", i)
		}
	}
}
