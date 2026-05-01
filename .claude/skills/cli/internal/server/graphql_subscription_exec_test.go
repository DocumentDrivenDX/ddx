package server

// TC-GQL-SUB-003: executionEvidence subscription streams log lines over WebSocket.
//
// Integration test that opens a real WebSocket client, sends an executionEvidence
// subscription, drives fake log data through a stub ExecLogProvider, and
// verifies that all stdout and stderr lines arrive as ExecutionEvents in order.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
	"github.com/gorilla/websocket"
)

// stubExecLogProvider is a fake ExecLogProvider. It returns an error until
// readyCh is closed, then returns the configured stdout/stderr.
type stubExecLogProvider struct {
	readyCh chan struct{}
	stdout  string
	stderr  string
}

func newStubExecLogProvider(stdout, stderr string) *stubExecLogProvider {
	s := &stubExecLogProvider{
		readyCh: make(chan struct{}),
		stdout:  stdout,
		stderr:  stderr,
	}
	close(s.readyCh) // immediately available
	return s
}

func (s *stubExecLogProvider) GetExecLog(_ string) (string, string, error) {
	select {
	case <-s.readyCh:
		return s.stdout, s.stderr, nil
	default:
		return "", "", fmt.Errorf("run not found")
	}
}

// TC-GQL-SUB-003: executionEvidence subscription delivers log lines via WebSocket.
func TestGraphQLExecutionEvidenceSubscription(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-exec-sub-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	// Stub returns 2 stdout lines and 1 stderr line.
	stub := newStubExecLogProvider("line one\nline two", "error line")

	_, dial := newGraphQLSubscriptionTestServer(t, &ddxgraphql.Resolver{
		State:      srv.state,
		WorkingDir: srv.WorkingDir,
		ExecLogs:   stub,
	})
	conn := dial()
	defer conn.Close()

	send := func(msg wsMsg) {
		t.Helper()
		data, _ := json.Marshal(msg)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			t.Fatalf("WebSocket write: %v", err)
		}
	}

	recv := func() wsMsg {
		t.Helper()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("WebSocket read: %v", err)
		}
		var msg wsMsg
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return msg
	}

	// graphql-transport-ws handshake.
	send(wsMsg{Type: "connection_init"})
	ack := recv()
	if ack.Type != "connection_ack" {
		t.Fatalf("expected connection_ack, got %q", ack.Type)
	}

	// Subscribe.
	queryPayload, _ := json.Marshal(map[string]string{
		"query": `subscription { executionEvidence(runID: "run-abc") { eventID runID stream line } }`,
	})
	send(wsMsg{ID: "1", Type: "subscribe", Payload: queryPayload})

	type eventData struct {
		EventID string `json:"eventID"`
		RunID   string `json:"runID"`
		Stream  string `json:"stream"`
		Line    string `json:"line"`
	}

	var received []eventData
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for subscription events; got %d so far", len(received))
		default:
		}
		msg := recv()
		switch msg.Type {
		case "next":
			var payload struct {
				Data struct {
					EE eventData `json:"executionEvidence"`
				} `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				t.Fatalf("unmarshal next payload: %v", err)
			}
			received = append(received, payload.Data.EE)
		case "complete":
			goto done
		case "ping":
			send(wsMsg{Type: "pong"})
		default:
			t.Logf("unexpected message type: %s", msg.Type)
		}
	}
done:

	// Expect 3 events: 2 stdout + 1 stderr.
	if len(received) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(received), received)
	}

	checks := []struct {
		stream string
		line   string
	}{
		{"stdout", "line one"},
		{"stdout", "line two"},
		{"stderr", "error line"},
	}
	for i, chk := range checks {
		got := received[i]
		if got.RunID != "run-abc" {
			t.Errorf("event[%d]: runID=%q want run-abc", i, got.RunID)
		}
		if got.Stream != chk.stream {
			t.Errorf("event[%d]: stream=%q want %q", i, got.Stream, chk.stream)
		}
		if got.Line != chk.line {
			t.Errorf("event[%d]: line=%q want %q", i, got.Line, chk.line)
		}
		if got.EventID == "" {
			t.Errorf("event[%d]: eventID is empty", i)
		}
	}
}
