package agent

import "time"

// RateLimitSignal is a normalized rate-limit snapshot parsed from harness
// HTTP response headers. All fields are optional; an unset integer field is
// represented by -1 so callers can distinguish "unknown" from "zero".
type RateLimitSignal struct {
	CeilingTokens        int       // -1 when unknown
	CeilingWindowSeconds int       // -1 when unknown
	Remaining            int       // -1 when unknown
	ResetAt              time.Time // zero when unknown
}

// HasAny reports whether any field is populated.
func (s RateLimitSignal) HasAny() bool {
	if s.CeilingTokens >= 0 {
		return true
	}
	if s.CeilingWindowSeconds >= 0 {
		return true
	}
	if s.Remaining >= 0 {
		return true
	}
	if !s.ResetAt.IsZero() {
		return true
	}
	return false
}
