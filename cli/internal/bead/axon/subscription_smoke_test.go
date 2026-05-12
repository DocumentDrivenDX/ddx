package axon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestAxonSubscription_ReceivesBeadMutations(t *testing.T) {
	projectDir := t.TempDir()
	store := bead.NewStore(projectDir + "/.ddx")
	require.NoError(t, store.Init(context.Background()))

	hub := bead.NewWatcherHub(10 * time.Millisecond)
	t.Cleanup(hub.Close)

	srv, handler := newAxonSmokeServer(t, projectDir, store, hub)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	client := NewClient(DualTransport{
		Queries:       httpQueryTransport{endpoint: ts.URL + "/graphql", client: ts.Client()},
		Subscriptions: NewWebSocketSubscriptionTransport(ts.URL + "/graphql"),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	stream, err := client.SubscribeChangeEvents(ctx, projectDir)
	require.NoError(t, err)

	select {
	case <-srv.subscribed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscription to register")
	}

	input := BeadInput{
		ID:           "axon-smoke-bead",
		Title:        "axion smoke bead",
		Status:       bead.StatusOpen,
		Priority:     bead.DefaultPriority,
		IssueType:    bead.DefaultType,
		Labels:       []string{},
		Dependencies: []DependencyInput{},
	}

	created, err := client.CreateBead(ctx, input)
	require.NoError(t, err)
	require.Equal(t, input.ID, created.ID)
	require.Equal(t, input.Title, created.Title)

	first := waitForChangeEvent(t, stream, 5*time.Second)
	require.Equal(t, input.ID, first.BeadID)
	require.Equal(t, "created", first.Kind)
	require.NotEmpty(t, first.EventID)

	updatedInput := BeadInputFromLocal(created.ToLocal())
	updatedInput.Title = "axion smoke bead updated"
	updatedInput.Status = bead.StatusInProgress

	updated, err := client.UpdateBead(ctx, created.ID, 1, updatedInput)
	require.NoError(t, err)
	require.Equal(t, updatedInput.Title, updated.Title)
	require.Equal(t, updatedInput.Status, updated.Status)

	second := waitForChangeEvent(t, stream, 5*time.Second)
	require.Equal(t, input.ID, second.BeadID)
	require.Equal(t, "status_changed", second.Kind)
	require.NotEqual(t, first.EventID, second.EventID)

	cancel()
}

type httpQueryTransport struct {
	endpoint string
	client   *http.Client
}

func (t httpQueryTransport) Query(ctx context.Context, query string, variables map[string]any, response any) error {
	body := map[string]any{"query": query}
	if variables != nil {
		body["variables"] = variables
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) == 0 {
			return fmt.Errorf("axon query transport: status %s", resp.Status)
		}
		return fmt.Errorf("axon query transport: status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		return fmt.Errorf("axon query error: %s", envelope.Errors[0].Message)
	}
	return json.Unmarshal(envelope.Data, response)
}

type axonSmokeServer struct {
	store      *bead.Store
	hub        *bead.WatcherHub
	upgrader   websocket.Upgrader
	subscribed chan struct{}
}

func newAxonSmokeServer(t *testing.T, projectDir string, store *bead.Store, hub *bead.WatcherHub) (*axonSmokeServer, http.Handler) {
	t.Helper()
	s := &axonSmokeServer{
		store: store,
		hub:   hub,
		upgrader: websocket.Upgrader{
			CheckOrigin:  func(*http.Request) bool { return true },
			Subprotocols: []string{graphqlWSSubprotocol},
		},
		subscribed: make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			s.handleSubscription(t, w, r, projectDir)
			return
		}
		s.handleQuery(t, w, r)
	})
	return s, mux
}

func (s *axonSmokeServer) handleQuery(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	defer r.Body.Close()

	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	write := func(payload any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": payload})
	}

	switch req.Query {
	case createBeadMutation:
		rawInput, _ := json.Marshal(req.Variables["input"])
		var input BeadInput
		if err := json.Unmarshal(rawInput, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		b := bead.Bead{
			ID:           input.ID,
			Title:        input.Title,
			Status:       input.Status,
			Priority:     input.Priority,
			IssueType:    input.IssueType,
			Owner:        input.Owner,
			Labels:       append([]string(nil), input.Labels...),
			Parent:       input.Parent,
			Description:  input.Description,
			Acceptance:   input.Acceptance,
			Notes:        input.Notes,
			Dependencies: dependencyInputsToLocal(input.Dependencies),
			Extra:        cloneStringAnyMap(input.Extra),
		}
		if err := s.store.Create(context.Background(), &b); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		write(map[string]any{"createEntity": BeadFromLocal(b)})
	case updateBeadMutation:
		id, _ := req.Variables["id"].(string)
		rawInput, _ := json.Marshal(req.Variables["input"])
		var input BeadInput
		if err := json.Unmarshal(rawInput, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mutate := func(b *bead.Bead) error {
			b.Title = input.Title
			b.Priority = input.Priority
			b.IssueType = input.IssueType
			b.Owner = input.Owner
			b.Labels = append([]string(nil), input.Labels...)
			b.Parent = input.Parent
			b.Description = input.Description
			b.Acceptance = input.Acceptance
			b.Notes = input.Notes
			b.Dependencies = dependencyInputsToLocal(input.Dependencies)
			b.Extra = cloneStringAnyMap(input.Extra)
			return nil
		}
		if err := s.store.UpdateWithLifecycleStatus(id, input.Status, bead.LifecycleTransitionOptions{}, mutate); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated, err := s.store.Get(context.Background(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		write(map[string]any{"updateEntity": BeadFromLocal(*updated)})
	case getBeadQuery:
		id, _ := req.Variables["id"].(string)
		b, err := s.store.Get(context.Background(), id)
		if err != nil {
			write(map[string]any{"ddxBead": nil})
			return
		}
		write(map[string]any{"ddxBead": BeadFromLocal(*b)})
	case listBeadsQuery:
		beads, err := s.store.ReadAll(context.Background())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out := make([]Bead, 0, len(beads))
		for _, b := range beads {
			out = append(out, BeadFromLocal(b))
		}
		write(map[string]any{"ddxBeads": out})
	default:
		http.Error(w, "unsupported query", http.StatusBadRequest)
	}
}

func (s *axonSmokeServer) handleSubscription(t *testing.T, w http.ResponseWriter, r *http.Request, projectDir string) {
	t.Helper()

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer conn.Close()

	var msg graphqlWSMessage
	if err := conn.ReadJSON(&msg); err != nil {
		return
	}
	if msg.Type != "connection_init" {
		return
	}
	if err := conn.WriteJSON(graphqlWSMessage{Type: "connection_ack"}); err != nil {
		return
	}

	if err := conn.ReadJSON(&msg); err != nil {
		return
	}
	if msg.Type != "subscribe" {
		return
	}

	var payload struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return
	}
	if payload.Query != changeEventsSubscription {
		return
	}
	projectID, _ := payload.Variables["projectID"].(string)
	if projectID != projectDir {
		return
	}

	subCh, unsub := s.hub.SubscribeLifecycle(projectID)
	defer unsub()
	close(s.subscribed)

	for {
		select {
		case evt, ok := <-subCh:
			if !ok {
				return
			}
			raw, err := json.Marshal(map[string]any{
				"data": map[string]any{
					"changeEvents": ChangeEventFromLifecycle(evt),
				},
			})
			if err != nil {
				return
			}
			if err := conn.WriteJSON(graphqlWSMessage{ID: msg.ID, Type: "next", Payload: raw}); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func waitForChangeEvent(t *testing.T, stream <-chan ChangeEvent, timeout time.Duration) ChangeEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case evt, ok := <-stream:
		if !ok {
			t.Fatal("subscription closed before expected event arrived")
		}
		return evt
	case <-ctx.Done():
		t.Fatalf("timed out waiting for change event: %v", ctx.Err())
		return ChangeEvent{}
	}
}

func dependencyInputsToLocal(src []DependencyInput) []bead.Dependency {
	if len(src) == 0 {
		return nil
	}
	out := make([]bead.Dependency, 0, len(src))
	for _, dep := range src {
		out = append(out, bead.Dependency{
			IssueID:     dep.IssueID,
			DependsOnID: dep.DependsOnID,
			Type:        dep.Type,
			CreatedAt:   dep.CreatedAt,
			CreatedBy:   dep.CreatedBy,
			Metadata:    dep.Metadata,
		})
	}
	return out
}
