# Shared CLI binary verification

Verified that the cmd and workerprobe integration tests already use package-level `sync.Once`-guarded shared binary builders instead of per-test `go build` calls.

## Evidence

- `cli/cmd/e2e_binary_test.go`
  - `getSmokeTestBinaryPath(t *testing.T)` builds the CLI once per package run.
  - `TestE2E_SharedCLIBinaryBuildsOnce` asserts the build counter remains `1`.
- `cli/internal/agent/workerprobe/probe_integration_test.go`
  - `sharedDdxBinary(t *testing.T)` builds the CLI once per package run.
  - `TestSharedDdxBinaryBuildsOnce` asserts the build counter remains `1`.

## Verification

- `cd cli && go test -run 'TestE2E|TestSharedDdxBinary' ./cmd`
- `cd cli && go test -run 'TestWorker_RealAttemptEvents_FlowToServer|TestSharedDdxBinaryBuildsOnce' ./internal/agent/workerprobe`

Both commands passed.
