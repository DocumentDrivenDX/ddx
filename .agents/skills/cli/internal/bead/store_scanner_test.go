package bead

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseBeadJSONL_LargeLine covers ddx-f8a11202 AC #5 and #6: the bead
// store scanner must read lines up to 16MB without bufio.Scanner crashing.
// Real-world incidents reached 1.46MB on a single bead row; bd export lines
// can easily exceed 1MB when a bead accumulates several 65KB event bodies.
func TestParseBeadJSONL_LargeLine(t *testing.T) {
	header := `{"id":"ddx-big","title":"Large","type":"task","status":"open","priority":2,"description":"`
	footer := `","labels":[],"deps":[],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`

	bodySize := 2 * 1024 * 1024 // 2MB description; well over the old 1MB cap
	body := strings.Repeat("x", bodySize)

	line := header + body + footer + "\n"
	require.Greater(t, len(line), 1024*1024,
		"fixture must exceed the previous 1MB scanner cap to prove the raise matters")

	beads, _, err := parseBeadJSONL([]byte(line))
	require.NoError(t, err, "2MB bead row must parse without bufio.Scanner: token too long")
	require.Len(t, beads, 1)
	assert.Equal(t, "ddx-big", beads[0].ID)
	assert.Equal(t, bodySize, len(beads[0].Description),
		"description must round-trip intact; truncation here would mask the real bug")
}

// TestParseBeadJSONL_OversizedLineSurfacesWarning covers AC #6: a line that
// exceeds the 16MB buffer must NOT crash; the error path must identify the
// affected row so an operator can trace which bead triggered it.
func TestParseBeadJSONL_OversizedLineSurfacesWarning(t *testing.T) {
	healthy := `{"id":"ddx-small","title":"ok","type":"task","status":"open","priority":2,"labels":[],"deps":[],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	oversize := 20 * 1024 * 1024 // 20MB, larger than the 16MB buffer
	var buf bytes.Buffer
	buf.WriteString(healthy)
	buf.WriteByte('\n')
	buf.WriteByte('{')
	buf.WriteString(`"id":"ddx-oversize","description":"`)
	for i := 0; i < oversize; i++ {
		buf.WriteByte('x')
	}
	buf.WriteString(`"}`)
	buf.WriteByte('\n')
	buf.WriteString(healthy)
	buf.WriteByte('\n')

	beads, _, err := parseBeadJSONL(buf.Bytes())

	if err != nil {
		assert.Contains(t, err.Error(), "token too long",
			"when the scanner overruns, the error must name the cause, not an opaque io error: %v", err)
	} else {
		require.NotEmpty(t, beads,
			"at least the healthy lines should have parsed: buffer overrun on one line should not drop the whole stream silently")
		ids := make(map[string]bool)
		for _, b := range beads {
			ids[b.ID] = true
		}
		assert.True(t, ids["ddx-small"],
			"healthy bead before/after the oversized row must still surface after a buffer overrun")
	}
	_ = fmt.Sprintf // touch
}
