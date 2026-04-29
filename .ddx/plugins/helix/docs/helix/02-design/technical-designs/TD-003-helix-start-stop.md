# Technical Design: TD-003 — helix start/stop

**Status**: draft
**Date**: 2026-04-05
**Story Reference**: hx-407ed8b8 | **Feature**: FEAT-002 | **Solution Design**: SD-001

## Problem

`helix run` currently blocks the terminal. The `helix-worker` skill launches
background runs from inside an agent session, but there is no discoverable CLI
command for operators to start and stop a background HELIX run directly. Users
must know about `helix run --summary &` and manually manage the process.

## Acceptance Criteria

1. `helix start` launches `helix run` as a background process.
2. A PID file is written at `${HELIX_PID_FILE:-.ddx/helix.pid}`.
3. `helix status` shows whether a run is active (PID alive check).
4. `helix stop` kills the background run cleanly.
5. Stale PID files are detected and cleaned up.

## Design

### helix start

```
helix start [--agent claude|codex] [--max-cycles N] [helix run options...]
```

Behavior:
1. Check if a run is already active (PID file exists, process alive) → refuse
   with an error.
2. Delegate to `helix run --summary <forwarded-flags>`.
3. Run the process in the background with output redirected to the log file.
4. Write the PID to `${tracker_dir}/helix.pid`.
5. Print: `helix: started (pid=<PID>, log=<path>)`

Implementation:
```bash
run_start() {
  local pid_file="${tracker_dir}/helix.pid"
  if _helix_pid_alive "$pid_file"; then
    local pid; pid="$(cat "$pid_file")"
    die "helix run already active (pid=%s). Use 'helix stop' first." "$pid"
  fi

  # Clean stale PID file
  rm -f "$pid_file"

  # Launch helix run in background with summary mode
  helix run --summary "$@" &
  local bg_pid=$!
  printf '%s\n' "$bg_pid" > "$pid_file"
  printf 'helix: started (pid=%s, log=%s)\n' "$bg_pid" "$log_file" >&2
}
```

### helix stop

```
helix stop
```

Behavior:
1. Read PID from `${tracker_dir}/helix.pid`.
2. If PID is alive, send SIGTERM.
3. Wait up to 10 seconds for graceful shutdown.
4. If still alive, send SIGKILL.
5. Remove the PID file.
6. Print: `helix: stopped (pid=<PID>)`

Implementation:
```bash
run_stop() {
  local pid_file="${tracker_dir}/helix.pid"
  if [[ ! -f "$pid_file" ]]; then
    die "no active helix run (no PID file)"
  fi
  local pid; pid="$(cat "$pid_file")"

  if ! kill -0 "$pid" 2>/dev/null; then
    rm -f "$pid_file"
    die "stale PID file removed (pid=%s was not running)" "$pid"
  fi

  kill "$pid" 2>/dev/null
  local waited=0
  while kill -0 "$pid" 2>/dev/null && (( waited < 10 )); do
    sleep 1
    (( waited++ ))
  done

  if kill -0 "$pid" 2>/dev/null; then
    kill -9 "$pid" 2>/dev/null || true
  fi
  rm -f "$pid_file"
  printf 'helix: stopped (pid=%s)\n' "$pid" >&2
}
```

### helix status integration

Add to `run_status()` after the "Last Run" section:

```bash
# Active run check
local pid_file="${tracker_dir}/helix.pid"
if _helix_pid_alive "$pid_file"; then
  local pid; pid="$(cat "$pid_file")"
  printf '\nActive Run:\n'
  printf '  PID:  %s\n' "$pid"
  printf '  Log:  %s\n' "$log_file"
else
  if [[ -f "$pid_file" ]]; then
    rm -f "$pid_file"  # clean up stale file
  fi
fi
```

JSON output adds `active_run: {pid: N, log: "path"} | null`.

### Helper

```bash
_helix_pid_alive() {
  local pid_file="$1"
  [[ -f "$pid_file" ]] || return 1
  local pid; pid="$(cat "$pid_file")"
  [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null
}
```

### Command surface

| Command | Behavior |
|---------|----------|
| `helix start [opts]` | Launch background run, write PID file |
| `helix stop` | Kill background run, remove PID file |
| `helix status` | Show active run if PID alive |

These mirror the skill surface: `helix-worker` (agent-side) maps to
`helix start` (CLI-side).

## Test Plan

Add to `tests/helix-cli.sh`:

1. `helix start` writes PID file and runs in background
2. `helix start` refuses when already running
3. `helix stop` kills process and removes PID file
4. `helix stop` handles stale PID file gracefully
5. `helix status` shows active run when PID alive
6. `helix status` cleans stale PID file

## Edge Cases

- **Double start**: Refused with error pointing to `helix stop`.
- **Stale PID file**: Detected via `kill -0` check; cleaned up automatically
  by both `start` and `status`.
- **Process tree cleanup**: `helix run` spawns agent subprocesses. SIGTERM to
  the parent triggers the existing `cleanup` trap which handles child process
  termination.
- **No tracker dir**: `helix start` calls `require_tracker` before writing PID.

## Dependencies

- Existing `helix run --summary` mode
- Existing log file infrastructure
- Existing cleanup trap in `run_helix`

## Migration

No migration needed. PID file is ephemeral and auto-cleaned.
