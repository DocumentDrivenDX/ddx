# Practices: go-std

## Requirements (Frame phase)
- Specify minimum Go version in `go.mod`
- Identify test levels needed: unit, integration (VCR/stubbed), functional (binary), E2E (live)
- If the project hits external HTTP APIs, plan for VCR recording in integration tests

## Design
- Package layout: `cmd/` for CLI entry points, `internal/` for application logic
- Define interfaces in the consumer package; return concrete types where practical
- Use minimal, consumer-driven interfaces
- Guard shared state explicitly; prefer immutable data; use `errgroup` for concurrent work
- Embed version metadata: `Version`, `BuildTime`, `GitCommit` via `-ldflags`

## Implementation
- Formatting: `go fmt ./...` (run before every commit — enforced by `gofmt -l .` check)
- Error wrapping: `fmt.Errorf("context: %w", err)` — always add context
- Sentinel errors: define with `errors.New` for expected conditions; compare with `errors.Is`
- Concurrency: pass `context.Context` first; use `errgroup.WithContext` for fan-out; avoid goroutine leaks
- Logging: structured with `log/slog` (stdlib) or project-chosen structured logger; no `fmt.Print*` in library code
- No `panic` outside startup; in `main()`, convert panics to fatal log + exit

## Testing
- Run with: `go test ./...`
- Race detection: `go test -race ./...` for concurrent code
- Build tags for test levels:
  - (no tag): fast unit tests, no external deps
  - `-tags=integration`: VCR playback, no live APIs
  - `-tags=functional`: built binary CLI tests
  - `-tags=e2e`: live API tests (requires credentials)
- Table-driven tests for pure functions
- HTTP stubs: VCR cassette recording (`VCR_MODE=record` to capture, `VCR_MODE=playback` for CI)
- Use `testify/assert` or `testify/require` for assertions; not bare `t.Fatal` comparisons
- Coverage: `go test -coverprofile=coverage.out ./...` + `go tool cover -func`; 80% threshold

## Quality Gates (pre-commit / CI)
- `go fmt ./...` (or `gofmt -l .` check)
- `go vet ./...`
- `golangci-lint run`
- `gosec ./...` (high severity/confidence)
- `govulncheck ./...`
- `go test -race ./...` (unit + integration tags)

## Dependency Management
- `go mod tidy` after any dependency change
- `go mod download` for reproducible builds
- No vendoring unless required by deployment constraints
- `govulncheck ./...` before releases
