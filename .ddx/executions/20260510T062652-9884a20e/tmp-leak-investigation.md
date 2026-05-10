# /tmp Leak Investigation — exec.Command Subprocesses

**Bead:** ddx-2b8170eb  
**Date:** 2026-05-10  
**Packages exercised:** `cli/cmd` (all `_test.go` files)

---

## Instrumentation Method

**Primary:** `strace -f -e trace=openat,open,creat,mkdir,mkdirat` wrapped around targeted `go test` invocations covering every test that the static audit flagged as containing a `/tmp` literal.  
**Secondary:** Static read of every call site plus function signature analysis to confirm no filesystem I/O path is exercised.

Commands run:
```
strace -f -e trace=openat,open,creat,mkdir,mkdirat \
  go test ./cmd/... \
  -run 'TestWorkDefaultOutput|TestWorkJSONOutput|TestWorkStopSummary|TestDocProse|TestServerStatePrune|TestAgentDoctorWorkers' \
  -count=1
```
Full suite (without strace, to avoid timeout) also passed: `go test ./cmd/... -count=1 -timeout=60s`.

---

## Static Analysis — /tmp References Inventoried

| File | Location | Pattern | Category |
|------|----------|---------|----------|
| `cli/cmd/testutils_test.go` | lines 80–90 | `tempBase := "/tmp"` fallback in `GetTestLibraryPath()` | Guarded by env-var override |
| `cli/cmd/work_test.go` | lines 310, 335, 356, 396, 425, 450, 460 | `writeExecuteLoopResult(&out, "/tmp/project", ...)` | Display-label arg, no I/O |
| `cli/cmd/agent_doctor_workers_test.go` | line 57 | `"project_root": "/tmp/projA"` | JSON fixture value, no I/O |
| `cli/cmd/doc_prose_test.go` | lines 34, 59, 96, 130, 167 | `url: file:///tmp/ddx-library` in YAML configs | Config URL never dereferenced |
| `cli/cmd/server_state_test.go` | lines 41, 147 | `/tmp/TestFooBar...`, `/tmp/TestFoo123/001` | Intentional phantom paths for prune test |

---

## Findings Per Site

### 1. `testutils_test.go` — `GetTestLibraryPath()` `/tmp` fallback (lines 80–90)

**Finding: No leak.**

`GetTestLibraryPath()` selects its base directory by checking `TMP` → `TMPDIR` → `/tmp`. `TestMain` (same file, line 34) calls `isolateCmdTestTempRoot()` before any test runs. That function:
1. Creates a process-scoped temp root: `os.MkdirTemp("", "ddx-cmd-tests-*")` → e.g. `/tmp/ddx-cmd-tests-4144046217`.
2. Overrides `TMPDIR` and `TMP` to that root.
3. Returns a cleanup that `os.RemoveAll`s the root after `m.Run()`.

As a result, the `/tmp` fallback is never reached: `GetTestLibraryPath()` resolves `TMP` first, so `.test-library` and `.test-library.git` are created under `/tmp/ddx-cmd-tests-<random>/`, not under raw `/tmp/`. Strace confirmed this: the actual paths created are `/tmp/ddx-cmd-tests-4144046217/.test-library` etc.

### 2. `testutils_test.go` — `testLibraryOnce` race window

**Finding: No cross-binary race exists.**

`testLibraryOnce` is a `sync.Once` declared in `testutils_test.go`, a `package cmd` test file. It is therefore compiled only into the `cmd` package test binary. No other package imports or calls `GetTestLibraryPath()` (grep confirmed: the only non-`testutils_test.go` reference is `e2e_smoke_test.go`, which is also in `package cmd`). There is no cross-binary sharing of this state; each concurrent `go test ./...` package binary that exists only runs the `cmd` package's tests in one process. The `sync.Once` guarantee is sufficient within that single binary.

The sister bead that removes the shared fixture pattern would eliminate even the theoretical risk, but the current code has no actual race.

### 3. `work_test.go` — `/tmp/project` in `writeExecuteLoopResult` calls (7 sites)

**Finding: No leak. Pure display-label.**

`writeExecuteLoopResult(w io.Writer, projectRoot string, ...)` uses `projectRoot` only as a display string:
- Text mode: `fmt.Fprintf(w, "\nproject: %s\n", projectRoot)` — writes to `w` (a `bytes.Buffer`).
- JSON mode: serialized as `"project_root"` field — written to `w` (a `bytes.Buffer`).

No filesystem stat, open, or mkdir operations are performed on the `/tmp/project` string. Strace confirmed: zero syscalls touching `/tmp/project`.

### 4. `agent_doctor_workers_test.go` — `"project_root": "/tmp/projA"` (line 57)

**Finding: No leak. Stub server fixture data.**

`/tmp/projA` is a JSON map literal in the stub HTTP server's response body. It populates a worker descriptor that the test asserts can be parsed and displayed. The path string is never stat'd or opened. Strace showed no access to `/tmp/projA`.

### 5. `doc_prose_test.go` — `url: file:///tmp/ddx-library` (5 sites)

**Finding: No leak. Config URL is never dereferenced by `doc prose`.**

Five tests write a `config.yaml` containing `url: file:///tmp/ddx-library` into a `t.TempDir()`. The `doc prose` command reads config to extract prose checker settings (mode, vocabulary, policy) but does not clone or sync the library URL during `RunE`. Strace confirmed: zero filesystem accesses to `/tmp/ddx-library` during these tests. The `GetTestLibraryPath()` fixture (the one actually used to seed the library) is accessed from the project fixture directory under the repo root.

**Minor quality note (non-blocking):** The hardcoded `file:///tmp/ddx-library` URL is a stale placeholder — it dates from before the `GetTestLibraryPath()` fixture was wired in. It is safe only because `doc prose` never loads the library at runtime. If that command were ever changed to access the library, these tests would silently misconfigure themselves. The fix would be replacing the literal URL with `"file://" + te.TestLibraryURL` via the `NewTestEnvironment` → `CreateDefaultConfig()` pattern. No bead needed now since there is no actual leak; it is noted here as a code health observation.

### 6. `server_state_test.go` — `/tmp/TestFooBar...` paths (lines 41, 147)

**Finding: No leak. Intentional phantom paths.**

These paths are used as test inputs to verify that the `server state prune` command removes projects whose paths no longer exist on disk. The prune logic does `os.Stat` on each path and removes entries where stat fails. `/tmp/TestFooBar<n>/<n>` are path strings that intentionally do not exist; the test asserts they are removed. No writes to these paths occur.

---

## Summary

**No additional /tmp leak surfaces confirmed beyond the existing `isolateCmdTestTempRoot` isolation mechanism.**

All six categories of `/tmp` literals found by static analysis are accounted for:

| Category | Verdict |
|----------|---------|
| `GetTestLibraryPath` fallback `/tmp` | Safe — `TestMain` overrides env vars before any test runs |
| `testLibraryOnce` cross-binary race | Non-issue — function is private to single test binary |
| `writeExecuteLoopResult` `/tmp/project` | Display label only, no I/O |
| `agent_doctor_workers_test` `/tmp/projA` | Stub JSON data, no I/O |
| `doc_prose_test` `file:///tmp/ddx-library` | Config URL never dereferenced at runtime |
| `server_state_test` phantom paths | Intentional non-existent paths for prune test |

No follow-on beads required. No inline fixes required.

---

## residual sync.Once note

The `testLibraryOnce` shared-fixture pattern is architecturally suboptimal (it survives between test cases and relies on env-var isolation rather than `t.TempDir()`). If the sister bead "tests: replace shared /tmp/.test-library fixture" lands, this risk is eliminated entirely. Verification after that bead: `grep -r 'testLibraryOnce\|GetTestLibraryPath' cli/cmd/` should return only historical uses, all removed.
