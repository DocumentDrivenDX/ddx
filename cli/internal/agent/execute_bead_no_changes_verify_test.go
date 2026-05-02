package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseNoChangesRationale(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want ParsedNoChangesRationale
	}{
		{
			name: "empty",
			in:   "",
			want: ParsedNoChangesRationale{Kind: NoChangesKindUnjustified},
		},
		{
			name: "bare",
			in:   "nothing to do here",
			want: ParsedNoChangesRationale{Kind: NoChangesKindUnjustified},
		},
		{
			name: "verification_command basic",
			in:   "verification_command: go test ./foo -run TestX\noutput: PASS",
			want: ParsedNoChangesRationale{Kind: NoChangesKindVerified, VerificationCommand: "go test ./foo -run TestX"},
		},
		{
			name: "needs_investigation with reason",
			in:   "status: needs_investigation\nreason: provider quota unknown\nmore detail line",
			want: ParsedNoChangesRationale{Kind: NoChangesKindNeedsInvestigation, NeedsInvestigationReason: "provider quota unknown more detail line"},
		},
		{
			name: "verification_command takes precedence over needs_investigation",
			in:   "status: needs_investigation\nverification_command: true",
			want: ParsedNoChangesRationale{Kind: NoChangesKindVerified, VerificationCommand: "true"},
		},
		{
			name: "case insensitive markers",
			in:   "Verification_Command: ls\noutput: x",
			want: ParsedNoChangesRationale{Kind: NoChangesKindVerified, VerificationCommand: "ls"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseNoChangesRationale(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDefaultVerificationCommandRunner(t *testing.T) {
	t.Run("exit 0", func(t *testing.T) {
		code, _, err := DefaultVerificationCommandRunner(context.Background(), "", "true")
		assert.NoError(t, err)
		assert.Equal(t, 0, code)
	})
	t.Run("exit non-zero", func(t *testing.T) {
		code, _, err := DefaultVerificationCommandRunner(context.Background(), "", "false")
		assert.NoError(t, err)
		assert.NotEqual(t, 0, code)
	})
}
