package server

// TC-GQL-SUB-001: workerProgress subscription streams real events over WebSocket.
//
// Integration test that opens a real WebSocket client, sends a
// workerProgress subscription, drives fake progress events through a
// stub ProgressSubscriber, and verifies all events arrive in order.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/DocumentDrivenDX/ddx/internal/agent"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
	"github.com/gorilla/websocket"
)

// newGraphQLHandlerWithWorkers creates an HTTP handler backed by the given
// server state but with a custom ProgressSubscriber injected — used in tests
// to drive fake progress events without a live worker.
func newGraphQLHandlerWithWorkers(srv *Server, sub ddxgraphql.ProgressSubscriber) http.Handler {
	gqlSrv := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
		Resolvers: &ddxgraphql.Resolver{
			State:      srv.state,
			WorkingDir: srv.WorkingDir,
			Workers:    sub,
		},
		Directives: ddxgraphql.DirectiveRoot{},
	}))
	gqlSrv.AddTransport(transport.POST{})
	gqlSrv.AddTransport(transport.GET{})
	gqlSrv.AddTransport(transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	})
	return gqlSrv
}

// stubProgressSubscriber is a fake ProgressSubscriber that returns a
// pre-loaded channel of events.
type stubProgressSubscriber struct {
	ch  chan agent.ProgressEvent
	sub func()
}

func (s *stubProgressSubscriber) SubscribeProgress(_ string) (<-chan agent.ProgressEvent, func()) {
	return s.ch, s.sub
}

// wsMsg is a minimal graphql-transport-ws protocol message.
type wsMsg struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// TC-GQL-SUB-001: workerProgress subscription delivers events via WebSocket.
func TestGraphQLWorkerProgressSubscription(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-sub-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	// Build a stub subscriber with 3 pre-loaded events.
	eventCh := make(chan agent.ProgressEvent, 8)
	now := time.Now().UTC()
	events := []agent.ProgressEvent{
		{EventID: "evt-1", WorkerID: "worker-abc", Phase: "loop.start", TS: now, Message: "worker started"},
		{EventID: "evt-2", WorkerID: "worker-abc", Phase: "bead.claimed", BeadID: "bead-xyz", TS: now.Add(time.Second)},
		{EventID: "evt-3", WorkerID: "worker-abc", Phase: "bead.result", BeadID: "bead-xyz", TS: now.Add(2 * time.Second), Message: "execution complete"},
	}
	for _, e := range events {
		eventCh <- e
	}
	close(eventCh)

	stub := &stubProgressSubscriber{
		ch:  eventCh,
		sub: func() {},
	}

	// Override the Workers field on the server's resolver via a test-only
	// handler that injects the stub.
	ts := httptest.NewServer(newGraphQLHandlerWithWorkers(srv, stub))
	defer ts.Close()

	// Connect via WebSocket using the graphql-transport-ws subprotocol.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/graphql"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, map[string][]string{
		"Sec-WebSocket-Protocol": {"graphql-transport-ws"},
	})
	if err != nil {
		t.Fatalf("WebSocket dial: %v", err)
	}
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
		"query": `subscription { workerProgress(workerID: "worker-abc") { eventID workerID phase logLine beadID } }`,
	})
	send(wsMsg{ID: "1", Type: "subscribe", Payload: queryPayload})

	// Collect events until "complete".
	type eventData struct {
		Data struct {
			WP struct {
				EventID  string  `json:"eventID"`
				WorkerID string  `json:"workerID"`
				Phase    string  `json:"phase"`
				LogLine  *string `json:"logLine"`
				BeadID   *string `json:"beadID"`
			} `json:"workerProgress"`
		} `json:"data"`
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
					WP struct {
						EventID  string  `json:"eventID"`
						WorkerID string  `json:"workerID"`
						Phase    string  `json:"phase"`
						LogLine  *string `json:"logLine"`
						BeadID   *string `json:"beadID"`
					} `json:"workerProgress"`
				} `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				t.Fatalf("unmarshal next payload: %v", err)
			}
			received = append(received, eventData{Data: payload.Data})
		case "complete":
			goto done
		case "ping":
			send(wsMsg{Type: "pong"})
		default:
			t.Logf("unexpected message type: %s", msg.Type)
		}
	}
done:

	if len(received) != 3 {
		t.Fatalf("expected 3 events, got %d", len(received))
	}

	checks := []struct {
		phase   string
		logLine *string
		beadID  *string
	}{
		{"loop.start", ptr("worker started"), nil},
		{"bead.claimed", nil, ptr("bead-xyz")},
		{"bead.result", ptr("execution complete"), ptr("bead-xyz")},
	}
	for i, chk := range checks {
		wp := received[i].Data.WP
		if wp.Phase != chk.phase {
			t.Errorf("event[%d]: phase=%q want %q", i, wp.Phase, chk.phase)
		}
		if wp.WorkerID != "worker-abc" {
			t.Errorf("event[%d]: workerID=%q want worker-abc", i, wp.WorkerID)
		}
		if chk.logLine != nil {
			if wp.LogLine == nil || *wp.LogLine != *chk.logLine {
				t.Errorf("event[%d]: logLine=%v want %v", i, wp.LogLine, chk.logLine)
			}
		} else if wp.LogLine != nil {
			t.Errorf("event[%d]: expected no logLine, got %q", i, *wp.LogLine)
		}
		if chk.beadID != nil {
			if wp.BeadID == nil || *wp.BeadID != *chk.beadID {
				t.Errorf("event[%d]: beadID=%v want %v", i, wp.BeadID, chk.beadID)
			}
		} else if wp.BeadID != nil {
			t.Errorf("event[%d]: expected no beadID, got %q", i, *wp.BeadID)
		}
	}
}

func ptr(s string) *string { return &s }
