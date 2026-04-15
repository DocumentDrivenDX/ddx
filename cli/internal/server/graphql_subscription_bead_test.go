package server

// TC-GQL-SUB-002: beadLifecycle subscription streams real events over WebSocket.
//
// Integration test that opens a real WebSocket client, sends a beadLifecycle
// subscription, drives fake lifecycle events through a stub
// BeadLifecycleSubscriber, and verifies events arrive in order.

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
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
	"github.com/gorilla/websocket"
)

// newGraphQLHandlerWithBeadBus creates an HTTP handler backed by the given
// server state but with a custom BeadLifecycleSubscriber injected — used in
// tests to drive fake lifecycle events without a live bead store.
func newGraphQLHandlerWithBeadBus(srv *Server, sub ddxgraphql.BeadLifecycleSubscriber) http.Handler {
	gqlSrv := handler.New(ddxgraphql.NewExecutableSchema(ddxgraphql.Config{
		Resolvers: &ddxgraphql.Resolver{
			State:      srv.state,
			WorkingDir: srv.WorkingDir,
			Workers:    srv.workers,
			BeadBus:    sub,
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

// stubBeadLifecycleSubscriber is a fake BeadLifecycleSubscriber that returns
// a pre-loaded channel of events.
type stubBeadLifecycleSubscriber struct {
	ch  chan bead.LifecycleEvent
	sub func()
}

func (s *stubBeadLifecycleSubscriber) SubscribeLifecycle(_ string) (<-chan bead.LifecycleEvent, func()) {
	return s.ch, s.sub
}

// TC-GQL-SUB-002: beadLifecycle subscription delivers events via WebSocket.
func TestGraphQLBeadLifecycleSubscription(t *testing.T) {
	xdgDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDir)
	t.Setenv("DDX_NODE_NAME", "gql-bead-sub-test-node")

	workDir := setupTestDir(t)
	srv := New(":0", workDir)

	// Build a stub subscriber with 3 pre-loaded events.
	eventCh := make(chan bead.LifecycleEvent, 8)
	now := time.Now().UTC()
	events := []bead.LifecycleEvent{
		{
			EventID:   "bx-aabbccdd-a1b2c3d4",
			BeadID:    "bx-aabbccdd",
			Kind:      "created",
			Summary:   "bead bx-aabbccdd created: implement feature X",
			Timestamp: now,
		},
		{
			EventID:   "bx-aabbccdd-e5f6a7b8",
			BeadID:    "bx-aabbccdd",
			Kind:      "status_changed",
			Summary:   "status changed from open to in_progress",
			Timestamp: now.Add(time.Second),
		},
		{
			EventID:   "bx-aabbccdd-c9d0e1f2",
			BeadID:    "bx-aabbccdd",
			Kind:      "status_changed",
			Summary:   "status changed from in_progress to closed",
			Timestamp: now.Add(2 * time.Second),
		},
	}
	for _, e := range events {
		eventCh <- e
	}
	close(eventCh)

	stub := &stubBeadLifecycleSubscriber{
		ch:  eventCh,
		sub: func() {},
	}

	ts := httptest.NewServer(newGraphQLHandlerWithBeadBus(srv, stub))
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
		"query": `subscription { beadLifecycle(projectID: "/test/project") { eventID beadID kind summary } }`,
	})
	send(wsMsg{ID: "1", Type: "subscribe", Payload: queryPayload})

	// Collect events until "complete".
	type eventData struct {
		EventID string  `json:"eventID"`
		BeadID  string  `json:"beadID"`
		Kind    string  `json:"kind"`
		Summary *string `json:"summary"`
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
					BL eventData `json:"beadLifecycle"`
				} `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				t.Fatalf("unmarshal next payload: %v", err)
			}
			received = append(received, payload.Data.BL)
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
		eventID string
		beadID  string
		kind    string
		summary string
	}{
		{"bx-aabbccdd-a1b2c3d4", "bx-aabbccdd", "created", "bead bx-aabbccdd created: implement feature X"},
		{"bx-aabbccdd-e5f6a7b8", "bx-aabbccdd", "status_changed", "status changed from open to in_progress"},
		{"bx-aabbccdd-c9d0e1f2", "bx-aabbccdd", "status_changed", "status changed from in_progress to closed"},
	}
	for i, chk := range checks {
		got := received[i]
		if got.EventID != chk.eventID {
			t.Errorf("event[%d]: eventID=%q want %q", i, got.EventID, chk.eventID)
		}
		if got.BeadID != chk.beadID {
			t.Errorf("event[%d]: beadID=%q want %q", i, got.BeadID, chk.beadID)
		}
		if got.Kind != chk.kind {
			t.Errorf("event[%d]: kind=%q want %q", i, got.Kind, chk.kind)
		}
		if got.Summary == nil || *got.Summary != chk.summary {
			t.Errorf("event[%d]: summary=%v want %q", i, got.Summary, chk.summary)
		}
	}
}
