package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxReviewerEventBody is the hard cap on bytes written to an event body for
// a reviewer verdict event. Full streams persist as a sidecar artifact; the
// body holds a summary plus the artifact path. 512 bytes fits a verdict name,
// one line of rationale, and a path without blowing the bead event envelope
// or tipping us toward bd's 65,535-byte per-field cap.
const maxReviewerEventBody = 512

// PersistReviewerStream writes the full reviewer raw output to an artifact
// sidecar under the project's execution evidence tree. Returns the relative
// path (from projectRoot) on success, empty string plus error on failure.
//
// The sidecar path shape is
// `.ddx/executions/<attemptID>/reviewer-stream.log` when attemptID is
// available; otherwise a timestamped fallback under
// `.ddx/executions/reviewer-streams/` keeps streams grouped even without a
// corresponding bundle. The caller is responsible for not losing the stream
// when this returns a non-nil error — log but do not drop the verdict.
func PersistReviewerStream(projectRoot, beadID, attemptID, fullStream string) (string, error) {
	if projectRoot == "" {
		return "", fmt.Errorf("reviewer stream artifact: empty projectRoot")
	}
	if fullStream == "" {
		return "", nil
	}

	var dir, filename string
	if attemptID != "" {
		dir = filepath.Join(projectRoot, ".ddx", "executions", attemptID)
		filename = "reviewer-stream.log"
	} else {
		dir = filepath.Join(projectRoot, ".ddx", "executions", "reviewer-streams")
		ts := time.Now().UTC().Format("20060102T150405")
		bid := beadID
		if bid == "" {
			bid = "unknown"
		}
		filename = ts + "-" + bid + ".log"
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("reviewer stream artifact: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(fullStream), 0o644); err != nil {
		return "", fmt.Errorf("reviewer stream artifact: write %s: %w", path, err)
	}

	rel, err := filepath.Rel(projectRoot, path)
	if err != nil {
		// Not fatal — return the absolute path so the reference still works.
		return path, nil
	}
	return rel, nil
}

// ReviewEventBody assembles a bounded event body for a reviewer verdict event
// from the verdict label, the rationale (which may be a full review stream),
// and the artifact path where the full stream was persisted. Guarantees the
// returned string is at most maxReviewerEventBody bytes.
//
// Layout:
//
//	<verdict>
//	<first non-empty rationale line, truncated to fit>
//	artifact: <artifactPath>
//
// artifactPath is optional; when absent the layout omits that line.
func ReviewEventBody(verdict, rationale, artifactPath string) string {
	verdict = strings.TrimSpace(verdict)
	rationale = strings.TrimSpace(rationale)
	artifactPath = strings.TrimSpace(artifactPath)

	var firstLine string
	if rationale != "" {
		for _, line := range strings.Split(rationale, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				firstLine = trimmed
				break
			}
		}
	}

	var b strings.Builder
	if verdict != "" {
		b.WriteString(verdict)
	}

	artifactLine := ""
	if artifactPath != "" {
		artifactLine = "artifact: " + artifactPath
	}

	// Reserve room for the verdict and the artifact line plus two newlines.
	reserved := b.Len()
	if artifactLine != "" {
		reserved += len("\n") + len(artifactLine)
	}
	budget := maxReviewerEventBody - reserved
	// Account for the newline before the rationale line.
	if firstLine != "" {
		budget -= len("\n")
	}
	if budget < 0 {
		budget = 0
	}

	if firstLine != "" {
		if len(firstLine) > budget {
			// Leave space for a trailing truncation marker.
			const marker = "…"
			if budget > len(marker) {
				firstLine = firstLine[:budget-len(marker)] + marker
			} else {
				firstLine = firstLine[:budget]
			}
		}
		if firstLine != "" {
			b.WriteString("\n")
			b.WriteString(firstLine)
		}
	}

	if artifactLine != "" {
		b.WriteString("\n")
		b.WriteString(artifactLine)
	}

	out := b.String()
	if len(out) > maxReviewerEventBody {
		out = out[:maxReviewerEventBody]
	}
	return out
}
