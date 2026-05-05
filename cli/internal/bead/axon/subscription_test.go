package axon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestAxonSubscription_ChangeEventsStream(t *testing.T) {
	t.Parallel()

	errCh := make(chan error, 1)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool { return true },
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
			return
		}
		defer conn.Close()

		fail := func(err error) {
			if err == nil {
				return
			}
			select {
			case errCh <- err:
			default:
			}
		}

		var msg graphqlWSMessage
		if err := conn.ReadJSON(&msg); err != nil {
			fail(err)
			return
		}
		if msg.Type != "connection_init" {
			fail(&unexpectedProtocolError{got: msg.Type, want: "connection_init"})
			return
		}

		if err := conn.WriteJSON(graphqlWSMessage{Type: "connection_ack"}); err != nil {
			fail(err)
			return
		}

		if err := conn.ReadJSON(&msg); err != nil {
			fail(err)
			return
		}
		if msg.Type != "subscribe" {
			fail(&unexpectedProtocolError{got: msg.Type, want: "subscribe"})
			return
		}

		var payload struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			fail(err)
			return
		}
		if payload.Query != changeEventsSubscription {
			fail(&unexpectedStringError{got: payload.Query, want: changeEventsSubscription})
			return
		}
		if got := payload.Variables["projectID"]; got != "project-123" {
			fail(&unexpectedStringError{got: toString(got), want: "project-123"})
			return
		}

		events := []ChangeEvent{
			{
				EventID:   "evt-1",
				BeadID:    "bead-1",
				Kind:      "created",
				Summary:   "bead created",
				Body:      "body-1",
				Actor:     "tester",
				Timestamp: time.Unix(10, 0).UTC(),
			},
			{
				EventID:   "evt-2",
				BeadID:    "bead-1",
				Kind:      "updated",
				Summary:   "bead updated",
				Body:      "body-2",
				Actor:     "tester",
				Timestamp: time.Unix(20, 0).UTC(),
			},
		}
		for _, evt := range events {
			payload, err := json.Marshal(map[string]any{
				"data": map[string]any{
					"changeEvents": evt,
				},
			})
			if err != nil {
				fail(err)
				return
			}
			if err := conn.WriteJSON(graphqlWSMessage{ID: "1", Type: "next", Payload: payload}); err != nil {
				fail(err)
				return
			}
		}
		if err := conn.WriteJSON(graphqlWSMessage{ID: "1", Type: "complete"}); err != nil {
			fail(err)
		}
	}))
	t.Cleanup(ts.Close)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	client := NewClient(DualTransport{
		Queries:       noopTransport{},
		Subscriptions: NewWebSocketSubscriptionTransport(wsURL),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	stream, err := client.SubscribeChangeEvents(ctx, "project-123")
	require.NoError(t, err)

	var got []ChangeEvent
	for evt := range stream {
		got = append(got, evt)
	}
	require.Len(t, got, 2)
	require.Equal(t, "evt-1", got[0].EventID)
	require.Equal(t, "bead-1", got[0].BeadID)
	require.Equal(t, "created", got[0].Kind)
	require.Equal(t, "bead created", got[0].Summary)
	require.Equal(t, "evt-2", got[1].EventID)
	require.Equal(t, "updated", got[1].Kind)

	select {
	case err := <-errCh:
		require.NoError(t, err)
	default:
	}
}

type unexpectedProtocolError struct {
	got  string
	want string
}

func (e *unexpectedProtocolError) Error() string {
	return "unexpected protocol message type: got " + e.got + " want " + e.want
}

type unexpectedStringError struct {
	got  string
	want string
}

func (e *unexpectedStringError) Error() string {
	return "unexpected string: got " + e.got + " want " + e.want
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		data, _ := json.Marshal(x)
		return string(data)
	}
}
