package evidence

// ClampOutput trims s to at most max bytes. When trimming occurs the
// returned text ends with TruncationMarker (the marker is included in
// the returned bytes; the length of the returned string may exceed max
// only by the marker length when max is too small to fit even the
// marker, in which case ClampOutput returns the bare marker truncated
// to max bytes). originalBytes is len(s) before clamping.
//
// FEAT-022 §1 (clamped text output, canonical truncation marker).
func ClampOutput(s string, max int) (clamped string, truncated bool, originalBytes int) {
	originalBytes = len(s)
	if max < 0 {
		max = 0
	}
	if originalBytes <= max {
		return s, false, originalBytes
	}
	// Reserve room for the marker. If max is smaller than the marker,
	// return a marker-only string truncated to max bytes (still flagged
	// truncated so the caller never silently drops content).
	if max <= len(TruncationMarker) {
		return TruncationMarker[:max], true, originalBytes
	}
	keep := max - len(TruncationMarker)
	return s[:keep] + TruncationMarker, true, originalBytes
}
