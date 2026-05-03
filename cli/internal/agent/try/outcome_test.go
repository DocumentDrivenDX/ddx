package try

import (
	"reflect"
	"testing"
	"time"
)

func TestParkReasonVocabulary(t *testing.T) {
	want := []ParkReason{
		"needs_review",
		"decomposition",
		"push_failed",
		"push_conflict",
		"cost_cap",
		"loop_error",
		"no_changes_unverified",
		"no_changes_unjustified",
		"rate_limit_budget_exhausted",
		"quota_paused",
		"lock_contention",
	}
	got := AllParkReasons()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AllParkReasons() = %v, want %v", got, want)
	}
	for _, r := range want {
		if !ParkReasonValid(r) {
			t.Errorf("ParkReasonValid(%q) = false, want true", r)
		}
		if r.String() != string(r) {
			t.Errorf("ParkReason(%q).String() = %q, want %q", r, r.String(), string(r))
		}
	}
	if ParkReasonValid("") {
		t.Errorf("ParkReasonValid(\"\") = true, want false")
	}
	if ParkReasonValid("not_a_real_reason") {
		t.Errorf("ParkReasonValid(unknown) = true, want false")
	}
}

func TestOutcome_RoundTripsExecuteBeadReport(t *testing.T) {
	base := Outcome{
		BeadID:     "ddx-aaaaaaaa",
		AttemptID:  "att-1",
		BaseRev:    "deadbeefdeadbeef",
		ResultRev:  "cafebabecafebabe",
		SessionID:  "sess-xyz",
		DurationMS: 4321,
		CostUSD:    0.1234,
		Cooldown:   90 * time.Second,
	}

	cases := []struct {
		name string
		o    Outcome
	}{
		{"merged", with(base, DispositionMerged, "")},
		{"already_done", with(base, DispositionAlreadyDone, "")},
		{"retry", with(base, DispositionRetry, "")},
		{"needs_human", with(base, DispositionNeedsHuman, "")},
		{"loop_error", with(base, DispositionLoopError, "")},
		{"park_needs_review", with(base, DispositionPark, ParkReasonNeedsReview)},
		{"park_decomposition", with(base, DispositionPark, ParkReasonDecomposition)},
		{"park_push_failed", with(base, DispositionPark, ParkReasonPushFailed)},
		{"park_push_conflict", with(base, DispositionPark, ParkReasonPushConflict)},
		{"park_cost_cap", with(base, DispositionPark, ParkReasonCostCap)},
		{"park_loop_error", with(base, DispositionPark, ParkReasonLoopError)},
		{"park_no_changes_unverified", with(base, DispositionPark, ParkReasonNoChangesUnverified)},
		{"park_no_changes_unjustified", with(base, DispositionPark, ParkReasonNoChangesUnjustified)},
		{"park_rate_limit", with(base, DispositionPark, ParkReasonRateLimitBudgetExhausted)},
		{"park_quota_paused", with(base, DispositionPark, ParkReasonQuotaPaused)},
		{"park_lock_contention", with(base, DispositionPark, ParkReasonLockContention)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := FromOutcome(tc.o)
			got := ToOutcome(rep)
			if !reflect.DeepEqual(got, tc.o) {
				t.Fatalf("round trip mismatch\n got: %+v\nwant: %+v\n via report: %+v", got, tc.o, rep)
			}
		})
	}
}

func with(o Outcome, d Disposition, p ParkReason) Outcome {
	o.Disposition = d
	o.ParkReason = p
	return o
}

func TestDisposition_String(t *testing.T) {
	for _, c := range []struct {
		d    Disposition
		want string
	}{
		{DispositionMerged, "merged"},
		{DispositionAlreadyDone, "already_done"},
		{DispositionRetry, "retry"},
		{DispositionPark, "park"},
		{DispositionNeedsHuman, "needs_human"},
		{DispositionLoopError, "loop_error"},
	} {
		if got := c.d.String(); got != c.want {
			t.Errorf("Disposition(%d).String() = %q, want %q", int(c.d), got, c.want)
		}
	}
}
