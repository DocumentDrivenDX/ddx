package federation

import (
	"errors"
	"testing"
)

func TestFederationForwardMutationContract_CarriesTargetAndOriginMetadata(t *testing.T) {
	expectedVersion := "rev-42"
	req := &ForwardMutationRequest{
		OriginIdentity:       "localhost:127.0.0.1:55812",
		ForwardingPath:       []string{"hub-node", "spoke-node"},
		RequestID:            "req-123",
		IdempotencyKey:       "idem-456",
		TargetNodeID:         "spoke-node",
		TargetProjectID:      "project-789",
		ExpectedVersion:      &expectedVersion,
		RequiredCapabilities: []string{"write"},
		Body:                 []byte(`{"query":"mutation { __typename }"}`),
		Headers: map[string]string{
			"X-DDx-Origin-Identity": "localhost:127.0.0.1:55812",
		},
	}

	if req.OriginIdentity != "localhost:127.0.0.1:55812" {
		t.Fatalf("origin identity = %q, want %q", req.OriginIdentity, "localhost:127.0.0.1:55812")
	}
	if got := len(req.ForwardingPath); got != 2 || req.ForwardingPath[0] != "hub-node" || req.ForwardingPath[1] != "spoke-node" {
		t.Fatalf("forwarding path = %+v, want [hub-node spoke-node]", req.ForwardingPath)
	}
	if req.RequestID != "req-123" {
		t.Fatalf("request id = %q, want %q", req.RequestID, "req-123")
	}
	if req.IdempotencyKey != "idem-456" {
		t.Fatalf("idempotency key = %q, want %q", req.IdempotencyKey, "idem-456")
	}
	if req.TargetNodeID != "spoke-node" {
		t.Fatalf("target node id = %q, want %q", req.TargetNodeID, "spoke-node")
	}
	if req.TargetProjectID != "project-789" {
		t.Fatalf("target project id = %q, want %q", req.TargetProjectID, "project-789")
	}
	if req.ExpectedVersion == nil || *req.ExpectedVersion != expectedVersion {
		t.Fatalf("expected version = %+v, want %q", req.ExpectedVersion, expectedVersion)
	}
	if len(req.RequiredCapabilities) != 1 || req.RequiredCapabilities[0] != "write" {
		t.Fatalf("required capabilities = %+v, want [write]", req.RequiredCapabilities)
	}
}

func TestFederationForwardMutationContract_DefinesTypedRefusals(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ForwardMutationRefusalKind
	}{
		{name: "offline", err: ErrForwardMutationOffline, want: ForwardMutationRefusalOffline},
		{name: "stale", err: ErrForwardMutationStale, want: ForwardMutationRefusalStale},
		{name: "read_only", err: ErrForwardMutationReadOnly, want: ForwardMutationRefusalReadOnly},
		{name: "missing-project", err: ErrForwardMutationMissingProject, want: ForwardMutationRefusalMissingProject},
		{name: "missing-owner", err: ErrForwardMutationMissingOwner, want: ForwardMutationRefusalMissingOwner},
		{name: "broadcast-like", err: ErrForwardMutationBroadcastLike, want: ForwardMutationRefusalBroadcastLike},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(&ForwardMutationRefusalError{Kind: tc.want}, tc.err) {
				t.Fatalf("errors.Is(kind=%q, %v) must be true", tc.want, tc.err)
			}
			var got *ForwardMutationRefusalError
			if !errors.As(tc.err, &got) {
				t.Fatalf("errors.As(%v) failed", tc.err)
			}
			if got.Kind != tc.want {
				t.Fatalf("refusal kind = %q, want %q", got.Kind, tc.want)
			}
			if got.Error() == "" {
				t.Fatalf("error string must not be empty")
			}
		})
	}

	if err := (&ForwardMutationRequest{TargetNodeID: "", TargetProjectID: "p", ForwardingPath: []string{"hub"}}).Validate(); !errors.Is(err, ErrForwardMutationMissingOwner) {
		t.Fatalf("Validate missing owner = %v, want missing-owner refusal", err)
	}
	if err := (&ForwardMutationRequest{TargetNodeID: "node", TargetProjectID: "", ForwardingPath: []string{"hub"}}).Validate(); !errors.Is(err, ErrForwardMutationMissingProject) {
		t.Fatalf("Validate missing project = %v, want missing-project refusal", err)
	}
	if err := (&ForwardMutationRequest{TargetNodeID: "node", TargetProjectID: "p"}).Validate(); !errors.Is(err, ErrForwardMutationBroadcastLike) {
		t.Fatalf("Validate missing forwarding path = %v, want broadcast-like refusal", err)
	}
}
