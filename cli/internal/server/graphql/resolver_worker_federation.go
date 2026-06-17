package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/federation"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	workerDispatchSelection  = `id state kind workers { id state kind }`
	workerLifecycleSelection = `id state kind`
)

type workerMutationGraphQLResponse struct {
	Data   map[string]json.RawMessage  `json:"data"`
	Errors []beadLifecycleGraphQLError `json:"errors"`
}

func (r *mutationResolver) workerForwardTargetByProjectID(projectID string) (*federation.SpokeRecord, error) {
	projectID = strings.TrimSpace(projectID)
	if r.Federation == nil || projectID == "" {
		return nil, nil
	}

	reg := federation.NewRegistry()
	for _, spoke := range r.Federation.Spokes() {
		if err := reg.UpsertSpoke(spoke); err != nil {
			return nil, workerRefusalError("WORKER_FORWARD_REGISTRY_INVALID", err.Error())
		}
	}

	target, err := federation.RouteMutationToProjectOwner(reg, projectID)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "multiple registered owners"):
			return nil, workerRefusalError("WORKER_TARGET_AMBIGUOUS", err.Error())
		case strings.Contains(err.Error(), "no registered spoke owns"):
			return nil, nil
		default:
			return nil, workerRefusalError("WORKER_TARGET_UNKNOWN", err.Error())
		}
	}
	if target == nil {
		return nil, nil
	}
	if strings.TrimSpace(target.NodeID) == "" {
		return nil, workerRefusalError("WORKER_TARGET_UNKNOWN", "federation: target node missing")
	}
	if strings.TrimSpace(target.NodeID) == strings.TrimSpace(r.NodeID) {
		return nil, nil
	}
	return target, nil
}

func (r *mutationResolver) workerForwardTargetByWorkerID(workerID string) (string, *federation.SpokeRecord, error) {
	if r.Federation == nil || r.State == nil || strings.TrimSpace(workerID) == "" {
		return "", nil, nil
	}
	worker, ok := r.State.GetWorkerGraphQL(workerID)
	if !ok || worker == nil {
		return "", nil, nil
	}
	projectID := r.projectIDForProjectRoot(worker.ProjectRoot)
	if projectID == "" {
		return "", nil, nil
	}
	target, err := r.workerForwardTargetByProjectID(projectID)
	return projectID, target, err
}

func (r *Resolver) projectIDForProjectRoot(projectRoot string) string {
	if r == nil || r.State == nil || strings.TrimSpace(projectRoot) == "" {
		return ""
	}
	for _, proj := range r.State.GetProjectSnapshots(false) {
		if proj == nil {
			continue
		}
		if sameWorkerProjectRoot(proj.Path, projectRoot) {
			return strings.TrimSpace(proj.ID)
		}
	}
	return ""
}

func sameWorkerProjectRoot(a, b string) bool {
	a = cleanWorkerProjectRoot(a)
	b = cleanWorkerProjectRoot(b)
	return a != "" && b != "" && a == b
}

func cleanWorkerProjectRoot(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if eval, err := filepath.EvalSymlinks(path); err == nil {
		path = eval
	}
	return filepath.Clean(path)
}

func (r *mutationResolver) forwardStartWorker(ctx context.Context, target *federation.SpokeRecord, input StartWorkerInput) (*WorkerDispatchResult, error) {
	body, err := r.workerForwardEnvelope("StartWorker", map[string]any{"input": input})
	if err != nil {
		return nil, err
	}
	resp, err := r.forwardWorkerMutation(ctx, target, input.ProjectID, body)
	if err != nil {
		return nil, err
	}
	var out WorkerDispatchResult
	if err := decodeWorkerForwardResponse(resp, "startWorker", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *mutationResolver) forwardStopWorker(ctx context.Context, target *federation.SpokeRecord, projectID, id string) (*WorkerLifecycleResult, error) {
	body, err := r.workerForwardEnvelope("StopWorker", map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	resp, err := r.forwardWorkerMutation(ctx, target, projectID, body)
	if err != nil {
		return nil, err
	}
	var out WorkerLifecycleResult
	if err := decodeWorkerForwardResponse(resp, "stopWorker", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *mutationResolver) workerForwardEnvelope(mutationName string, variables map[string]any) ([]byte, error) {
	query, err := workerMutationDocument(mutationName)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, fmt.Errorf("graphql: marshal forwarded worker mutation body: %w", err)
	}
	return body, nil
}

func workerMutationDocument(mutationName string) (string, error) {
	switch mutationName {
	case "StartWorker":
		return `mutation StartWorker($input: StartWorkerInput!) {
  startWorker(input: $input) {
    ` + workerDispatchSelection + `
  }
}`, nil
	case "StopWorker":
		return `mutation StopWorker($id: ID!) {
  stopWorker(id: $id) {
    ` + workerLifecycleSelection + `
  }
}`, nil
	default:
		return "", fmt.Errorf("graphql: unsupported forwarded worker mutation %q", mutationName)
	}
}

func (r *mutationResolver) forwardWorkerMutation(ctx context.Context, target *federation.SpokeRecord, projectID string, body []byte) (*federation.ForwardMutationResponse, error) {
	if r.Federation == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}
	if target == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}

	httpReq := httpRequestFromContext(ctx)
	originIdentity := r.beadLifecycleRequestIdentity(httpReq)
	if originIdentity == "" {
		originIdentity = "unknown"
	}

	req := &federation.ForwardMutationRequest{
		OriginIdentity:       originIdentity,
		ForwardingPath:       []string{r.beadLifecycleCoordinatorIdentity(httpReq), target.NodeID},
		RequestID:            r.beadLifecycleRequestID(httpReq),
		IdempotencyKey:       r.beadLifecycleIdempotencyKey(httpReq),
		TargetNodeID:         target.NodeID,
		TargetProjectID:      strings.TrimSpace(projectID),
		RequiredCapabilities: []string{"write"},
		Body:                 body,
		Headers:              r.beadLifecycleRequestHeaders(httpReq),
	}
	resp, err := r.Federation.ForwardMutation(ctx, req)
	if err != nil {
		return nil, workerForwardError(err)
	}
	if resp == nil {
		return nil, fmt.Errorf("graphql: forwarded worker mutation returned no response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("graphql: forwarded worker mutation returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func decodeWorkerForwardResponse(resp *federation.ForwardMutationResponse, responseField string, out any) error {
	if resp == nil {
		return fmt.Errorf("graphql: forwarded worker mutation returned no body")
	}
	var parsed workerMutationGraphQLResponse
	if err := json.Unmarshal(resp.Body, &parsed); err != nil {
		return fmt.Errorf("graphql: decode forwarded worker mutation response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		first := parsed.Errors[0]
		if len(first.Extensions) > 0 {
			return &gqlerror.Error{Message: first.Message, Extensions: first.Extensions}
		}
		return fmt.Errorf("%s", first.Message)
	}
	raw, ok := parsed.Data[responseField]
	if !ok {
		return fmt.Errorf("graphql: forwarded worker mutation missing %s payload", responseField)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("graphql: decode forwarded worker payload: %w", err)
	}
	return nil
}

func workerForwardError(err error) error {
	switch {
	case errors.Is(err, federation.ErrForwardMutationMissingOwner):
		return workerRefusalError("WORKER_TARGET_NOT_OWNED", err.Error())
	case errors.Is(err, federation.ErrForwardMutationBroadcastLike):
		return workerRefusalError("WORKER_TARGET_AMBIGUOUS", err.Error())
	case errors.Is(err, federation.ErrForwardMutationReadOnly):
		return workerRefusalError("WORKER_TARGET_READ_ONLY", err.Error())
	case errors.Is(err, federation.ErrForwardMutationOffline):
		return workerRefusalError("WORKER_TARGET_OFFLINE", err.Error())
	case errors.Is(err, federation.ErrForwardMutationStale):
		return workerRefusalError("WORKER_TARGET_STALE", err.Error())
	default:
		return fmt.Errorf("graphql: forward worker mutation: %w", err)
	}
}

func workerRefusalError(code, message string) error {
	return &gqlerror.Error{
		Message: message,
		Extensions: map[string]any{
			"code": code,
		},
	}
}
