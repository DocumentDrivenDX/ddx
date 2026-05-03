package bead

import (
	"strings"
	"testing"
)

// TestChecksBypasses_NilBeadAndEmptyExtra confirms the nil-safe contract:
// callers must be able to invoke ChecksBypasses without first checking
// bead.Extra themselves.
func TestChecksBypasses_NilBeadAndEmptyExtra(t *testing.T) {
	got, err := ChecksBypasses(nil)
	if err != nil || got != nil {
		t.Fatalf("nil bead must return (nil, nil); got (%v, %v)", got, err)
	}
	got, err = ChecksBypasses(&Bead{})
	if err != nil || got != nil {
		t.Fatalf("empty Extra must return (nil, nil); got (%v, %v)", got, err)
	}
	got, err = ChecksBypasses(&Bead{Extra: map[string]any{"unrelated": 42}})
	if err != nil || got != nil {
		t.Fatalf("Extra without checks_bypass must return (nil, nil); got (%v, %v)", got, err)
	}
}

// TestChecksBypasses_ParseValid verifies the happy path: a JSON-decoded array
// of {name, reason, bead} round-trips through Extra into typed entries.
func TestChecksBypasses_ParseValid(t *testing.T) {
	b := &Bead{
		Extra: map[string]any{
			"checks_bypass": []any{
				map[string]any{
					"name":   "go-vet",
					"reason": "vendored upstream code; vet noise we cannot fix",
					"bead":   "ddx-deadbeef",
				},
				map[string]any{
					"name":   "spell-check",
					"reason": "intentional product term",
				},
			},
		},
	}
	got, err := ChecksBypasses(b)
	if err != nil {
		t.Fatalf("expected no error; got %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries; got %d", len(got))
	}
	if got[0].Name != "go-vet" || got[0].Bead != "ddx-deadbeef" {
		t.Fatalf("entry 0 mismatch: %+v", got[0])
	}
	if got[1].Name != "spell-check" || got[1].Bead != "" {
		t.Fatalf("entry 1 mismatch: %+v", got[1])
	}
}

// TestChecksBypasses_RejectsMissingReason covers AC #3: a checks_bypass
// without a reason must be rejected loudly so a check cannot be silently
// disabled.
func TestChecksBypasses_RejectsMissingReason(t *testing.T) {
	cases := []struct {
		name string
		raw  any
	}{
		{
			name: "reason absent",
			raw: []any{
				map[string]any{"name": "go-vet"},
			},
		},
		{
			name: "reason empty string",
			raw: []any{
				map[string]any{"name": "go-vet", "reason": ""},
			},
		},
		{
			name: "reason whitespace only",
			raw: []any{
				map[string]any{"name": "go-vet", "reason": "   \t\n"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &Bead{Extra: map[string]any{"checks_bypass": tc.raw}}
			_, err := ChecksBypasses(b)
			if err == nil {
				t.Fatal("expected error for missing/empty reason; got nil")
			}
			if !strings.Contains(err.Error(), "reason") {
				t.Fatalf("error must mention 'reason'; got %q", err.Error())
			}
		})
	}
}

// TestChecksBypasses_RejectsMissingName guards against a bypass entry that
// names no check — without a name there is nothing to skip.
func TestChecksBypasses_RejectsMissingName(t *testing.T) {
	b := &Bead{Extra: map[string]any{
		"checks_bypass": []any{
			map[string]any{"reason": "some reason"},
		},
	}}
	_, err := ChecksBypasses(b)
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected error mentioning 'name'; got %v", err)
	}
}

// TestChecksBypasses_RejectsMalformed ensures a non-array value at the
// checks_bypass key surfaces a helpful parse error rather than panicking.
func TestChecksBypasses_RejectsMalformed(t *testing.T) {
	b := &Bead{Extra: map[string]any{"checks_bypass": "not an array"}}
	_, err := ChecksBypasses(b)
	if err == nil {
		t.Fatal("expected parse error for non-array value; got nil")
	}
}
