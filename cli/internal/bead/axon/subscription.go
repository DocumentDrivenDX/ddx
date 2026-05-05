package axon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/gorilla/websocket"
)

const graphqlWSSubprotocol = "graphql-transport-ws"

const changeEventsSubscription = `subscription ChangeEvents($projectID: ID!) { changeEvents(projectID: $projectID) { ...ChangeEventFields } }`

// ChangeEvent mirrors the GraphQL subscription payload for lifecycle updates.
type ChangeEvent struct {
	EventID   string    `json:"eventID"`
	BeadID    string    `json:"beadID"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	Body      string    `json:"body"`
	Actor     string    `json:"actor"`
	Timestamp time.Time `json:"timestamp"`
}

// ChangeEventFromLifecycle converts the local bead lifecycle event into the
// Axon GraphQL subscription shape.
func ChangeEventFromLifecycle(src bead.LifecycleEvent) ChangeEvent {
	return ChangeEvent{
		EventID:   src.EventID,
		BeadID:    src.BeadID,
		Kind:      src.Kind,
		Summary:   src.Summary,
		Body:      src.Body,
		Actor:     src.Actor,
		Timestamp: src.Timestamp,
	}
}

// ToLifecycleEvent converts the GraphQL subscription payload back into the
// local bead lifecycle model.
func (c ChangeEvent) ToLifecycleEvent() bead.LifecycleEvent {
	return bead.LifecycleEvent{
		EventID:   c.EventID,
		BeadID:    c.BeadID,
		Kind:      c.Kind,
		Summary:   c.Summary,
		Body:      c.Body,
		Actor:     c.Actor,
		Timestamp: c.Timestamp,
	}
}

// SubscriptionTransport is the minimal GraphQL-over-WebSocket transport used
// by the Axon client subscription path.
type SubscriptionTransport interface {
	Subscribe(ctx context.Context, query string, variables map[string]any) (<-chan json.RawMessage, error)
}

// DualTransport lets callers combine a query transport and a subscription
// transport behind the same Client value.
type DualTransport struct {
	Queries       Transport
	Subscriptions SubscriptionTransport
}

// Query forwards GraphQL queries and mutations to the query transport.
func (d DualTransport) Query(ctx context.Context, query string, variables map[string]any, response any) error {
	if d.Queries == nil {
		return fmt.Errorf("axon: query transport not configured")
	}
	return d.Queries.Query(ctx, query, variables, response)
}

// Subscribe forwards GraphQL subscriptions to the subscription transport.
func (d DualTransport) Subscribe(ctx context.Context, query string, variables map[string]any) (<-chan json.RawMessage, error) {
	if d.Subscriptions == nil {
		return nil, fmt.Errorf("axon: subscription transport not configured")
	}
	return d.Subscriptions.Subscribe(ctx, query, variables)
}

// WebSocketSubscriptionTransport speaks the graphql-transport-ws protocol.
type WebSocketSubscriptionTransport struct {
	URL         string
	Header      http.Header
	Dialer      *websocket.Dialer
	InitPayload map[string]any
}

// NewWebSocketSubscriptionTransport creates a websocket subscription transport
// rooted at url. The URL may be ws://, wss://, http://, or https://.
func NewWebSocketSubscriptionTransport(url string) *WebSocketSubscriptionTransport {
	return &WebSocketSubscriptionTransport{URL: url}
}

// Query is unsupported on the websocket subscription transport.
func (t *WebSocketSubscriptionTransport) Query(context.Context, string, map[string]any, any) error {
	return fmt.Errorf("axon: websocket subscription transport does not execute queries")
}

// Subscribe opens a GraphQL WebSocket subscription and streams raw payloads
// from graphql-transport-ws next messages.
func (t *WebSocketSubscriptionTransport) Subscribe(ctx context.Context, query string, variables map[string]any) (<-chan json.RawMessage, error) {
	url := normalizeWebSocketURL(strings.TrimSpace(t.URL))
	if url == "" {
		return nil, fmt.Errorf("axon: websocket subscription url is required")
	}

	dialer := websocket.DefaultDialer
	if t.Dialer != nil {
		dialer = t.Dialer
	}
	if len(dialer.Subprotocols) == 0 {
		dialer = cloneDialer(dialer)
		dialer.Subprotocols = []string{graphqlWSSubprotocol}
	}

	header := cloneHeader(t.Header)
	conn, _, err := dialer.DialContext(ctx, url, header)
	if err != nil {
		return nil, fmt.Errorf("axon: websocket dial: %w", err)
	}

	closed := make(chan struct{})
	var closeOnce sync.Once
	cleanup := func() {
		closeOnce.Do(func() {
			close(closed)
			_ = conn.Close()
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			cleanup()
		case <-closed:
		}
	}()

	if conn.Subprotocol() != "" && conn.Subprotocol() != graphqlWSSubprotocol {
		cleanup()
		return nil, fmt.Errorf("axon: websocket subprotocol mismatch: got %q want %q", conn.Subprotocol(), graphqlWSSubprotocol)
	}

	if err := conn.WriteJSON(graphqlWSMessage{Type: "connection_init", Payload: mustJSON(t.InitPayload)}); err != nil {
		cleanup()
		return nil, fmt.Errorf("axon: websocket connection_init: %w", err)
	}

	for {
		var msg graphqlWSMessage
		if err := conn.ReadJSON(&msg); err != nil {
			cleanup()
			return nil, fmt.Errorf("axon: websocket ack: %w", err)
		}
		switch msg.Type {
		case "connection_ack":
			goto subscribed
		case "ping":
			_ = conn.WriteJSON(graphqlWSMessage{Type: "pong"})
		case "error":
			cleanup()
			return nil, fmt.Errorf("axon: websocket connection error")
		}
	}

subscribed:
	subPayload, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("axon: websocket subscribe payload: %w", err)
	}
	if err := conn.WriteJSON(graphqlWSMessage{ID: "1", Type: "subscribe", Payload: subPayload}); err != nil {
		cleanup()
		return nil, fmt.Errorf("axon: websocket subscribe: %w", err)
	}

	out := make(chan json.RawMessage)
	go func() {
		defer close(out)
		defer cleanup()
		for {
			var msg graphqlWSMessage
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			switch msg.Type {
			case "next":
				if len(msg.Payload) == 0 {
					continue
				}
				payload := append(json.RawMessage(nil), msg.Payload...)
				select {
				case out <- payload:
				case <-ctx.Done():
					return
				}
			case "complete":
				return
			case "ping":
				_ = conn.WriteJSON(graphqlWSMessage{Type: "pong"})
			case "error":
				return
			}
		}
	}()

	return out, nil
}

type graphqlWSMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type changeEventsEnvelope struct {
	Data struct {
		ChangeEvents *ChangeEvent `json:"changeEvents,omitempty"`
	} `json:"data"`
}

func normalizeWebSocketURL(url string) string {
	switch {
	case strings.HasPrefix(url, "http://"):
		return "ws://" + strings.TrimPrefix(url, "http://")
	case strings.HasPrefix(url, "https://"):
		return "wss://" + strings.TrimPrefix(url, "https://")
	default:
		return url
	}
}

func cloneHeader(src http.Header) http.Header {
	if len(src) == 0 {
		return http.Header{}
	}
	dst := make(http.Header, len(src))
	for k, values := range src {
		dst[k] = append([]string(nil), values...)
	}
	return dst
}

func cloneDialer(src *websocket.Dialer) *websocket.Dialer {
	if src == nil {
		return &websocket.Dialer{}
	}
	cp := *src
	cp.Subprotocols = append([]string(nil), src.Subprotocols...)
	return &cp
}

func mustJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// SubscribeChangeEvents streams change events for the given project ID.
func (c *Client) SubscribeChangeEvents(ctx context.Context, projectID string) (<-chan ChangeEvent, error) {
	sub, ok := c.transport.(SubscriptionTransport)
	if !ok {
		return nil, fmt.Errorf("axon: subscription transport not configured")
	}

	raw, err := sub.Subscribe(ctx, changeEventsSubscription, map[string]any{"projectID": projectID})
	if err != nil {
		return nil, err
	}

	out := make(chan ChangeEvent)
	go func() {
		defer close(out)
		for payload := range raw {
			var env changeEventsEnvelope
			if err := json.Unmarshal(payload, &env); err != nil {
				continue
			}
			if env.Data.ChangeEvents == nil {
				continue
			}
			select {
			case out <- *env.Data.ChangeEvents:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}
