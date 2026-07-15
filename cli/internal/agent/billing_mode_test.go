package agent

import (
	"fmt"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
)

func TestBillingPresentationModeUsesPublicFizeauValues(t *testing.T) {
	cases := []struct {
		billing string
		want    string
	}{
		{billing: string(agentlib.BillingModelPerToken), want: BillingModePaid},
		{billing: string(agentlib.BillingModelSubscription), want: BillingModeSubscription},
		{billing: string(agentlib.BillingModelFixed), want: BillingModeLocal},
		{billing: "", want: BillingModeUnknown},
		{billing: "future-billing-model", want: BillingModeUnknown},
	}
	for _, tc := range cases {
		if got := BillingPresentationMode(tc.billing); got != tc.want {
			t.Fatalf("BillingPresentationMode(%q) = %q, want %q", tc.billing, got, tc.want)
		}
	}
}

func TestProviderIdentityDoesNotAffectBilling(t *testing.T) {
	logDir := t.TempDir()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	wantModes := map[string]string{}
	wantRaw := map[string]string{}
	identities := []struct {
		harness  string
		provider string
		model    string
		endpoint string
		baseURL  string
		surface  string
	}{
		{harness: "codex", provider: "openai", model: "gpt-5", endpoint: "openai-primary", baseURL: "https://api.openai.com/v1", surface: "codex"},
		{harness: "agent", provider: "local", model: "qwen-local", endpoint: "local-primary", baseURL: "http://127.0.0.1:1234/v1", surface: "openai-compat"},
		{harness: "unrecognized", provider: "unrecognized", model: "unknown-model", endpoint: "unknown-endpoint", baseURL: "https://example.invalid", surface: "unknown-surface"},
	}
	for i, identity := range identities {
		routing := &agentlib.ServiceRoutingDecisionData{
			Harness: identity.harness, Provider: identity.provider, Model: identity.model, Endpoint: identity.endpoint,
			Candidates: []agentlib.ServiceRoutingDecisionCandidate{{
				Harness: identity.harness, Provider: identity.provider, Model: identity.model, Endpoint: identity.endpoint,
				Eligible: true, Billing: agentlib.BillingModelSubscription,
			}},
		}
		candidate, ok := selectedRoutingCandidate(routing, &agentlib.ServiceRoutingActual{
			Harness: identity.harness, Provider: identity.provider, Model: identity.model,
		})
		if !ok {
			t.Fatalf("identity %+v did not resolve its exact public Fizeau candidate", identity)
		}
		entry := SessionIndexEntry{
			Harness: identity.harness, Provider: identity.provider, BaseURL: identity.baseURL,
			Model: identity.model, Surface: identity.surface, Billing: string(candidate.Billing),
		}
		if got := BillingPresentationMode(entry.Billing); got != BillingModeSubscription {
			t.Fatalf("identity %+v changed Fizeau billing to %q", identity, got)
		}
		subscriptionID := fmt.Sprintf("identity-%d-subscription", i)
		entry.ID = subscriptionID
		entry.StartedAt = now.Add(time.Duration(i*2) * time.Minute)
		if err := AppendSessionIndex(logDir, entry, entry.StartedAt); err != nil {
			t.Fatalf("persist subscription identity %+v: %v", identity, err)
		}
		wantModes[subscriptionID] = BillingModeSubscription
		wantRaw[subscriptionID] = string(agentlib.BillingModelSubscription)

		routing.Candidates[0].Billing = ""
		candidate, ok = selectedRoutingCandidate(routing, &agentlib.ServiceRoutingActual{
			Harness: identity.harness, Provider: identity.provider, Model: identity.model,
		})
		if !ok {
			t.Fatalf("identity %+v did not resolve its candidate with unknown billing", identity)
		}
		entry.Billing = string(candidate.Billing)
		if got := BillingPresentationMode(entry.Billing); got != BillingModeUnknown {
			t.Fatalf("identity %+v inferred billing %q without Fizeau evidence", identity, got)
		}
		unknownID := fmt.Sprintf("identity-%d-unknown", i)
		entry.ID = unknownID
		entry.StartedAt = now.Add(time.Duration(i*2+1) * time.Minute)
		if err := AppendSessionIndex(logDir, entry, entry.StartedAt); err != nil {
			t.Fatalf("persist unknown identity %+v: %v", identity, err)
		}
		wantModes[unknownID] = BillingModeUnknown
		wantRaw[unknownID] = ""
	}
	rows, err := ReadSessionIndex(logDir, SessionIndexQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(wantModes) {
		t.Fatalf("persisted identity rows=%d, want %d", len(rows), len(wantModes))
	}
	for _, row := range rows {
		if row.Billing != wantRaw[row.ID] || row.BillingMode != wantModes[row.ID] {
			t.Fatalf("stored row %q billing=%q mode=%q, want raw=%q mode=%q", row.ID, row.Billing, row.BillingMode, wantRaw[row.ID], wantModes[row.ID])
		}
	}
}

func TestValidateBillingMode(t *testing.T) {
	for _, mode := range []string{BillingModeUnknown, BillingModePaid, BillingModeSubscription, BillingModeLocal} {
		if !ValidateBillingMode(mode) {
			t.Fatalf("ValidateBillingMode(%q) = false, want true", mode)
		}
	}
	if ValidateBillingMode("free") {
		t.Fatal("ValidateBillingMode(\"free\") = true, want false")
	}
}
