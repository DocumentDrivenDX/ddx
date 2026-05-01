package server

// TC-GQL-SUB-004: coordinatorMetrics subscription streams metrics updates over WebSocket.
//
// Integration test that opens a real WebSocket client, sends a coordinatorMetrics
// subscription, drives changing metrics snapshots through a stub
// CoordinatorMetricsProvider, and verifies that updates arrive when metrics change.

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
	"github.com/gorilla/websocket"
)

// stubCoordinatorMetricsProvider is a fake CoordinatorMetricsProvider whose
// GetCoordinatorMetrics returns the current atomic snapshot. Tests swap the
// snapshot between calls to simulate coordinator updates.
type stubCoordinatorMetricsProvider struct {
	current atomic.Pointer[ddxgraphql.CoordinatorMetricsSnap]
}

func (s *stubCoordinatorMetricsProvider) GetCoordinatorMetrics(_ string) *ddxgraphql.CoordinatorMetricsSnap {
	return s.current.Load()
}

// TC-GQL-SUB-004: coordinatorMetrics subscription delivers updates via WebSocket.
func TestGraphQLCoordinatorMetricsSubscription(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-coord-sub-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	stub := &stubCoordinatorMetricsProvider{}
	// Start with nil (no coordinator yet) — the subscription should not emit
	// until we populate the first snapshot.
	snap1 := &ddxgraphql.CoordinatorMetricsSnap{
		Landed:          1,
		Preserved:       0,
		Failed:          0,
		PushFailed:      0,
		TotalDurationMS: 500,
		TotalCommits:    3,
	}
	snap2 := &ddxgraphql.CoordinatorMetricsSnap{
		Landed:          2,
		Preserved:       1,
		Failed:          0,
		PushFailed:      0,
		TotalDurationMS: 1200,
		TotalCommits:    7,
	}

	// Use a very short poll interval so the test runs in milliseconds.
	pollInterval := 30 * time.Millisecond

	_, dial := newGraphQLSubscriptionTestServer(t, &ddxgraphql.Resolver{
		State:               srv.state,
		WorkingDir:          srv.WorkingDir,
		CoordMetrics:        stub,
		MetricsPollInterval: pollInterval,
	})
	conn := dial()
	defer conn.Close()

	send := func(msg wsMsg) {
		t.Helper()
		data, _ := json.Marshal(msg)
		if werr := conn.WriteMessage(websocket.TextMessage, data); werr != nil {
			t.Fatalf("WebSocket write: %v", werr)
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
		"query": `subscription {
			coordinatorMetrics(projectRoot: "/test/project") {
				updateID projectRoot landed preserved failed pushFailed totalDurationMs totalCommits
			}
		}`,
	})
	send(wsMsg{ID: "1", Type: "subscribe", Payload: queryPayload})

	type updateData struct {
		UpdateID        string `json:"updateID"`
		ProjectRoot     string `json:"projectRoot"`
		Landed          int    `json:"landed"`
		Preserved       int    `json:"preserved"`
		Failed          int    `json:"failed"`
		PushFailed      int    `json:"pushFailed"`
		TotalDurationMs int    `json:"totalDurationMs"`
		TotalCommits    int    `json:"totalCommits"`
	}

	collectNext := func(timeout time.Duration) (updateData, bool) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return updateData{}, false
			default:
			}
			conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			_, data, err := conn.ReadMessage()
			if err != nil {
				// deadline hit — context will expire and return false
				continue
			}
			var msg wsMsg
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "next":
				var payload struct {
					Data struct {
						CM updateData `json:"coordinatorMetrics"`
					} `json:"data"`
				}
				if err := json.Unmarshal(msg.Payload, &payload); err != nil {
					t.Fatalf("unmarshal next payload: %v", err)
				}
				return payload.Data.CM, true
			case "ping":
				send(wsMsg{Type: "pong"})
			}
		}
	}

	// Initially nil — no events should arrive yet.
	// Set snap1 and wait for it to appear.
	stub.current.Store(snap1)

	got1, ok := collectNext(3 * time.Second)
	if !ok {
		t.Fatal("timed out waiting for first coordinatorMetrics update")
	}
	if got1.ProjectRoot != "/test/project" {
		t.Errorf("update1: projectRoot=%q want /test/project", got1.ProjectRoot)
	}
	if got1.Landed != 1 {
		t.Errorf("update1: landed=%d want 1", got1.Landed)
	}
	if got1.TotalCommits != 3 {
		t.Errorf("update1: totalCommits=%d want 3", got1.TotalCommits)
	}
	if got1.UpdateID == "" {
		t.Error("update1: updateID is empty")
	}

	// Change to snap2 — a second update should arrive with new values.
	stub.current.Store(snap2)

	got2, ok := collectNext(3 * time.Second)
	if !ok {
		t.Fatal("timed out waiting for second coordinatorMetrics update")
	}
	if got2.Landed != 2 {
		t.Errorf("update2: landed=%d want 2", got2.Landed)
	}
	if got2.Preserved != 1 {
		t.Errorf("update2: preserved=%d want 1", got2.Preserved)
	}
	if got2.TotalCommits != 7 {
		t.Errorf("update2: totalCommits=%d want 7", got2.TotalCommits)
	}
}
