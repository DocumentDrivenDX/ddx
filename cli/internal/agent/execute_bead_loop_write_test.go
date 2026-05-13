package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type atomicSinkCapture struct {
	mu         sync.Mutex
	buf        bytes.Buffer
	writes     int
	blockFirst int
	ready      chan struct{}
	release    chan struct{}
}

func newAtomicSinkCapture(blockFirst int) *atomicSinkCapture {
	return &atomicSinkCapture{
		blockFirst: blockFirst,
		ready:      make(chan struct{}),
		release:    make(chan struct{}),
	}
}

func (w *atomicSinkCapture) Write(p []byte) (int, error) {
	w.mu.Lock()
	_, _ = w.buf.Write(p)
	w.writes++
	shouldBlock := w.writes <= w.blockFirst
	if w.writes == w.blockFirst {
		close(w.ready)
	}
	w.mu.Unlock()

	if shouldBlock {
		<-w.release
	}
	return len(p), nil
}

func (w *atomicSinkCapture) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func TestWriteLoopEventWritesAtomicallyToWorkerProbe(t *testing.T) {
	sink := newAtomicSinkCapture(2)

	var wg sync.WaitGroup
	start := make(chan struct{})
	types := []string{"cleanup.tick", "progress.tick"}
	for _, eventType := range types {
		eventType := eventType
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			writeLoopEvent(sink, "sess-1", eventType, map[string]any{
				"source": eventType,
			}, time.Unix(0, 0))
		}()
	}

	close(start)

	select {
	case <-sink.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first two sink writes")
	}

	snapshot := sink.String()
	lines := strings.Split(strings.TrimSpace(snapshot), "\n")
	require.Len(t, lines, len(types), "each loop event should arrive as a single JSONL frame")
	for i, line := range lines {
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &got))
		require.Equal(t, "sess-1", got["session_id"], "line %d: wrong session_id", i)
		gotType, ok := got["type"].(string)
		require.True(t, ok, "line %d: missing event type", i)
		require.Contains(t, types, gotType, "line %d: wrong event type", i)
	}

	close(sink.release)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for writeLoopEvent goroutines to finish")
	}
}
