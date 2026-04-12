---
ddx:
  id: ADR-001
  depends_on:
    - helix.prd
---
# ADR-001: Go Development Best Practices

**Status:** Accepted
**Date:** 2026-04-04
**Context:** DDx, dun, and the DDx server are all Go projects. This ADR standardizes tooling and practices across the stack.

## Decision

Adopt the following Go development practices for all DDx ecosystem projects.

### 1. Code Formatting

| Tool | Purpose | When |
|------|---------|------|
| **gofumpt** | Strict superset of gofmt (no empty lines at block boundaries, etc.) | CI + editor |
| **goimports** | Auto-manage imports | Editor (via gopls) |
| **golangci-lint fmt** | Chain formatters in CI | CI |

Configure gopls as the editor formatter. In CI, run `gofumpt -l .` and fail on diff.

golangci-lint v2 has a dedicated `formatters` section:
```yaml
formatters:
  enable:
    - gofumpt
    - goimports
```

### 2. Linting

Use **golangci-lint v2** (v2.11+). Minimum enabled linters:

```yaml
version: "2"
linters:
  default: none
  enable:
    - staticcheck    # gold standard static analysis
    - govet          # official Go vet checks
    - errcheck       # unchecked errors
    - gosec          # security issues (CWE-mapped)
    - revive         # flexible replacement for golint
    - gocyclo        # cyclomatic complexity (threshold: 15)
    - misspell       # typos in comments/strings
    - unconvert      # unnecessary type conversions
    - unparam        # unused function parameters
    - gosimple       # code simplification
```

Existing DDx projects using golangci-lint v1 configs should run `golangci-lint migrate` to convert.

### 3. Security Scanning

Layer three tools — no single scanner catches everything:

| Tool | What It Catches | Database |
|------|----------------|----------|
| **govulncheck** | Known CVEs in dependencies (call-graph-aware — only reports vulns your code actually calls) | Go Vulnerability DB |
| **gosec** | Code-level security issues (injection, crypto, permissions) — 50+ rules mapped to CWEs | CWE rules |
| **trivy** | Container and binary scanning for module vulnerabilities | NVD, GitHub Advisories |

Run in CI:
```bash
govulncheck ./...                     # dependency CVEs
gosec ./...                           # code-level CWEs (also via golangci-lint)
trivy fs .                            # filesystem scan
```

govulncheck supports SARIF output for GitHub Code Scanning integration.

### 4. Dependency Pinning

- **go.mod** with minimum version selection (MVS) — the Go default
- **go.sum** committed to version control — cryptographic integrity verification
- **go mod tidy** enforced in CI (fail if `go mod tidy` produces a diff)
- **go mod verify** in CI — verify checksums match
- Go 1.24+ `tool` directives in go.mod for executable dependencies (replaces `tools.go` hack)

For SBOM generation, use **syft** — the de facto standard for Go projects.

### 5. Test Coverage

**Measurement:**
```bash
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out          # per-function breakdown
```

**Threshold enforcement** with vladopajic/go-test-coverage:
```yaml
# .testcoverage.yml
threshold:
  file: 50
  package: 60
  total: 70
```

**Target:** 70-80% total coverage. Focus on important code paths. Don't chase 100%.

**Integration test coverage** (Go 1.20+): build with `go build -cover`, run the binary, merge with `go tool covdata`.

### 6. Unit Testing

**Patterns:**
- **Table-driven tests** with `t.Run()` subtests — the idiomatic Go pattern
- **t.Parallel()** for independent tests — faster CI
- **t.Cleanup()** for teardown (not `defer` in subtests)
- **t.Helper()** in test helper functions for correct line reporting
- **testdata/** directory for fixtures (ignored by Go tooling)
- **testify** (assert/require/mock) — the standard third-party testing library

**Required CI flags:**
```bash
go test -race -shuffle=on -count=1 ./...
```
- `-race`: always detect race conditions
- `-shuffle=on`: catch implicit test ordering dependencies
- `-count=1`: disable test caching in CI

**Fuzzing** (built-in since Go 1.18): use for parsers, serialization, and input handling. Failing inputs auto-saved to `testdata/fuzz/` as permanent regression tests.

### 7. Profiling

**Development profiling:**
```go
import _ "net/http/pprof"  // expose on a separate, non-public port
```

```bash
go tool pprof http://localhost:6060/debug/pprof/heap
go test -cpuprofile=cpu.out -memprofile=mem.out -bench=.
```

Profile types: CPU, Heap, Goroutine, Block, Mutex.

**Continuous profiling** for production: **Grafana Pyroscope** or **Parca** (both open source). Enable version-to-version regression detection.

### 8. Observability and Telemetry

**Structured logging:** Use **log/slog** (standard library, Go 1.21+). It replaces `log`, `zap`, and `zerolog` for new projects.

```go
slog.Info("request handled",
    "method", r.Method,
    "path", r.URL.Path,
    "duration_ms", elapsed.Milliseconds(),
)
```

**Distributed tracing and metrics:** Use **OpenTelemetry Go SDK**.

| Signal | Status (2026) |
|--------|--------------|
| Traces | Stable |
| Metrics | Stable |
| Logs | Beta (stabilizing) |

Key packages:
- `go.opentelemetry.io/otel` — core API
- `go.opentelemetry.io/otel/sdk` — SDK implementation
- `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` — HTTP auto-instrumentation
- **otelslog** bridge — injects trace/span IDs into slog records

Architecture: App → OTel SDK → OTel Collector → backends (Jaeger, Prometheus, Grafana).

For DDx CLI tools, slog with JSON handler is sufficient. For the DDx server, add OpenTelemetry tracing on HTTP endpoints.

### 9. CI/CD (GitHub Actions)

Standard workflow structure:

```yaml
name: CI
on: [push, pull_request]
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: golangci/golangci-lint-action@v6
        with: { version: v2.11.4 }

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go test -race -shuffle=on -coverprofile=coverage.out ./...
      - uses: vladopajic/go-test-coverage@v2
        with: { config: .testcoverage.yml }

  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: golang/govulncheck-action@v1
        with: { output-format: sarif, output-file: govulncheck.sarif }
      - uses: github/codeql-action/upload-sarif@v3
        with: { sarif_file: govulncheck.sarif }
```

Key: `go-version-file: go.mod` keeps CI aligned with the project's Go version.

## Consequences

- All Go projects in the DDx ecosystem (ddx, dun) use the same tooling
- CI catches formatting, lint, security, and coverage issues before merge
- Security scanning is layered (CVEs + CWEs + container scanning)
- Structured logging with slog is the default, with OTel tracing for the server
- Coverage thresholds prevent regression without demanding 100%

## Alternatives Considered

- **zerolog/zap** for logging: mature but slog is now standard library and sufficient for our needs
- **golint** for linting: deprecated, replaced by revive
- **go test -v** in CI: too verbose; use `-race -shuffle=on` without `-v` for clean output
- **100% coverage target**: unrealistic and leads to low-value tests; 70-80% with focus on critical paths is more pragmatic
