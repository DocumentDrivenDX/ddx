package graphql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// BenchmarkArtifactsSearch_500Fixture builds the 500-artifact fixture defined
// in TD-029 (Artifact Full-Text Search & Facet Contract) and measures the p95
// latency of a `q` substring query against it.
//
// The fixture composition mirrors the TD-029 "Performance budget" table:
// 200 small md, 100 medium md, 50 large md, 25 oversize md, 75 PNG, 50 tar.
// Binary-classed files (PNG, tar) carry .ddx.yaml sidecars marked with the
// matching media types so collectSidecarArtifacts records them.
//
// This bead (Story 6 B4a) lands the benchmark harness only. Resolver-level
// body search is added in B4b; until then this benchmark measures the
// title∪path baseline against the same fixture so B4b regressions are
// visible against an apples-to-apples baseline.
func BenchmarkArtifactsSearch_500Fixture(b *testing.B) {
	root := buildBench500Fixture(b)
	r := &Resolver{WorkingDir: root}
	qr := &queryResolver{r}
	ctx := context.Background()

	// "ipsum" is sprinkled across textual fixture bodies and titles
	// (see writeMarkdownFixture) so the search produces non-zero hits.
	q := "ipsum"

	// Warm fs cache and verify the resolver returns matches before we
	// start timing — a cold first call dominates p95 otherwise and a
	// silent zero-match search would make the benchmark meaningless.
	if conn, err := qr.Artifacts(ctx, "bench", nil, nil, nil, nil, nil, &q, nil, nil, nil, nil); err != nil {
		b.Fatalf("warmup: %v", err)
	} else if conn.TotalCount == 0 {
		b.Fatalf("warmup: expected matches for %q, got 0", q)
	}

	durations := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_, err := qr.Artifacts(ctx, "bench", nil, nil, nil, nil, nil, &q, nil, nil, nil, nil)
		d := time.Since(start)
		if err != nil {
			b.Fatalf("iter %d: %v", i, err)
		}
		durations = append(durations, d)
	}
	b.StopTimer()

	if len(durations) == 0 {
		return
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	pct := func(p float64) time.Duration {
		idx := int(float64(len(durations)-1) * p)
		return durations[idx]
	}
	b.ReportMetric(float64(pct(0.50).Microseconds()), "p50_us/op")
	b.ReportMetric(float64(pct(0.95).Microseconds()), "p95_us/op")
	b.ReportMetric(float64(pct(0.99).Microseconds()), "p99_us/op")
}

// buildBench500Fixture writes 500 artifacts under <tmp>/.ddx/plugins/ and
// returns the project root. The directory is cleaned up by testing.B.
func buildBench500Fixture(tb testing.TB) string {
	tb.Helper()
	root := tb.TempDir()
	pluginDir := filepath.Join(root, ".ddx", "plugins", "bench")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		tb.Fatal(err)
	}

	rng := rand.New(rand.NewSource(1)) // deterministic fixture

	type spec struct {
		count     int
		minBytes  int
		maxBytes  int
		mediaType string
		ext       string
		writer    func(path string, size int, rng *rand.Rand) error
	}
	specs := []spec{
		{200, 1 << 10, 8 << 10, "text/markdown", ".md", writeMarkdownFixture},
		{100, 8 << 10, 64 << 10, "text/markdown", ".md", writeMarkdownFixture},
		{50, 64 << 10, 256 << 10, "text/markdown", ".md", writeMarkdownFixture},
		{25, 1 << 20, 4 << 20, "text/markdown", ".md", writeMarkdownFixture},
		{75, 4 << 10, 32 << 10, "image/png", ".png", writeBinaryFixture},
		{50, 32 << 10, 256 << 10, "application/x-tar", ".tar", writeBinaryFixture},
	}

	idx := 0
	for si, s := range specs {
		for i := 0; i < s.count; i++ {
			idx++
			size := s.minBytes
			if s.maxBytes > s.minBytes {
				size += rng.Intn(s.maxBytes - s.minBytes)
			}
			name := fmt.Sprintf("artifact-%03d-%d-%d%s", idx, si, i, s.ext)
			artifactPath := filepath.Join(pluginDir, name)
			if err := s.writer(artifactPath, size, rng); err != nil {
				tb.Fatal(err)
			}
			// Compute hash so generated_by.source_hash matches → "fresh".
			data, err := os.ReadFile(artifactPath)
			if err != nil {
				tb.Fatal(err)
			}
			h := sha256.Sum256(data)
			sidecar := fmt.Sprintf(`ddx:
  id: BENCH-%04d
  title: "Artifact ipsum %d (%s)"
  media_type: %s
  description: "Lorem ipsum dolor sit amet, sample artifact %d for the bench fixture."
  generated_by:
    run_id: bench-run
    prompt_summary: "bench fixture"
    source_hash: %s
`, idx, idx, s.mediaType, s.mediaType, idx, hex.EncodeToString(h[:]))
			if err := os.WriteFile(artifactPath+".ddx.yaml", []byte(sidecar), 0o644); err != nil {
				tb.Fatal(err)
			}
		}
	}
	return root
}

// writeMarkdownFixture writes a deterministic markdown file of approximately
// `size` bytes with embedded "ipsum" tokens that the q="ipsum" search hits.
func writeMarkdownFixture(path string, size int, rng *rand.Rand) error {
	const para = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. " +
		"Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. " +
		"Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris. "
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "# Bench Artifact (ipsum %d)\n\n", rng.Int()); err != nil {
		return err
	}
	written := 0
	for written < size {
		n, err := f.WriteString(para)
		if err != nil {
			return err
		}
		written += n
	}
	return nil
}

// writeBinaryFixture writes a deterministic pseudo-binary blob with NUL bytes
// scattered through it so a content-sniff classifier flags it as binary.
func writeBinaryFixture(path string, size int, rng *rand.Rand) error {
	buf := make([]byte, size)
	if _, err := rng.Read(buf); err != nil {
		return err
	}
	// Inject NULs at regular intervals to make the binary-sniff
	// rule (NUL byte in first 512 bytes) trigger reliably.
	for i := 0; i < len(buf); i += 64 {
		buf[i] = 0
	}
	return os.WriteFile(path, buf, 0o644)
}
