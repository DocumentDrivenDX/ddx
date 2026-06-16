#!/bin/sh
# server-managed-workers-scenario.sh
#
# Runs INSIDE an isolated container (see server-managed-workers-docker.sh). It
# proves that server-managed worker cleanup leaves no `ddx work`, fake claude,
# fake codex, shell, or sleep descendants behind across every cleanup path:
# explicit stop, double stop, watchdog reap, and server shutdown.
#
# It requires only: a `ddx` binary on PATH, git, and a POSIX `ps`/`pgrep`.
# It does NOT require Claude/Codex credentials, host tmux, or host process
# state — fake claude/codex binaries are placed first on PATH. The fakes return
# immediately for model-discovery/version probes (so routing does not hang) and
# only spawn long-lived sleeps when invoked as the actual agent with the
# --ddx-agent-run sentinel, faithfully modelling a working agent subprocess.
#
# Exit 0 => every cleanup path left zero leaks. Non-zero => a leak was found
# (the offending processes are printed).
set -u

REPO=/work/repo
BIN=/work/bin
export HOME=/root
export DDX_SERVER_URL=https://127.0.0.1:7743
PORT=7743
FAILED=0

# Distinctive sleep markers so process assertions cannot collide with anything
# unrelated in the container: 987001/987002 are the fake agents' own sleeps,
# 987011/987012 their child sleeps, 987003 the harness shell's foreground sleep.
MARKER='sleep 9870'

log()  { echo "[scenario] $*"; }
ok()   { echo "OK[$1]: $2"; }
fail() { echo "FAIL[$1]: $2"; FAILED=1; }

# leak_lines prints every still-running managed worker / fake agent / descendant.
# shellcheck disable=SC2009 # need pgid + full args columns; pgrep cannot show them
leak_lines() {
	ps -o pid,pgid,args 2>/dev/null \
		| grep -E "server-managed-worker-id|${BIN}/claude|${BIN}/codex|${MARKER}" \
		| grep -v grep
}

# wait_clean polls until the worker tree is gone (or a timeout), so the no-leak
# verdict is taken after cleanup has had a chance to run, not mid-teardown.
wait_clean() {
	i=0
	while [ "$i" -lt 20 ]; do
		[ -z "$(leak_lines)" ] && return 0
		i=$((i + 1)); sleep 1
	done
	return 1
}

assert_no_leak() {
	label="$1"
	wait_clean
	bad="$(leak_lines)"
	if [ -n "$bad" ]; then
		fail "$label" "leaked processes after cleanup:"
		echo "$bad"
	else
		ok "$label" "no managed worker / fake claude / fake codex / shell / sleep descendants remain"
	fi
}

# count_bead_stopped counts bead.stopped tracker events across all on-disk bead
# state (inline beads.jsonl and externalized attachments), robust to storage.
count_bead_stopped() {
	grep -rho 'bead.stopped' "$REPO/.ddx" 2>/dev/null | wc -l | tr -d ' '
}

worker_pid() { pgrep -f 'server-managed-worker-id' 2>/dev/null | head -1; }

# current_worker_id returns the newest worker-* dir (names are timestamp-sorted).
current_worker_id() {
	newest=""
	for d in "$REPO"/.ddx/workers/worker-*; do
		[ -d "$d" ] || continue
		newest="$(basename "$d")"
	done
	echo "$newest"
}

wait_fakes() {
	i=0
	while [ "$i" -lt 40 ]; do
		if [ -n "$(worker_pid)" ] && pgrep -f "$MARKER" >/dev/null 2>&1; then
			return 0
		fi
		i=$((i + 1)); sleep 1
	done
	return 1
}

write_config() { # $1 = watchdog/stall deadline
	printf 'version: "1.0"\nserver:\n  watchdog_deadline: %s\n  stall_deadline: %s\n' "$1" "$1" \
		> "$REPO/.ddx/config.yaml"
}

start_server() { # $1 = logfile
	( cd "$REPO" && ddx server --addr 127.0.0.1 --port "$PORT" --no-tsnet >"$1" 2>&1 ) &
	SRV=$!
	i=0
	while [ "$i" -lt 30 ]; do
		ddx worker status --json >/dev/null 2>&1 && return 0
		kill -0 "$SRV" 2>/dev/null || { echo "server exited early:"; cat "$1"; return 1; }
		i=$((i + 1)); sleep 1
	done
	echo "server did not become ready:"; cat "$1"
	return 1
}

stop_server() {
	[ -n "${SRV:-}" ] || return 0
	kill -TERM "$SRV" 2>/dev/null
	i=0
	while [ "$i" -lt 20 ]; do
		kill -0 "$SRV" 2>/dev/null || break
		i=$((i + 1)); sleep 1
	done
	SRV=""
}

############################ one-time fixture setup ############################
mkdir -p "$BIN" "$REPO"

cat > "$BIN/claude" <<'SH'
#!/bin/sh
# fake claude: fast for discovery/version probes; long-running only as the agent
case " $* " in
	*" --ddx-agent-run "*) sleep 987011 & sleep 987001 ;;
	*) exit 0 ;;
esac
SH
cat > "$BIN/codex" <<'SH'
#!/bin/sh
case " $* " in
	*" --ddx-agent-run "*) sleep 987012 & sleep 987002 ;;
	*) exit 0 ;;
esac
SH
chmod +x "$BIN/claude" "$BIN/codex"
export PATH="$BIN:$PATH"

cd "$REPO" || exit 2
git init -q
git config user.email scenario@ddx.test
git config user.name scenario
git config commit.gpgsign false
mkdir -p .ddx/workers .ddx/run-state

# The script-harness directive spawns the fake agents (in the worker's process
# group) and then holds the attempt open with a foreground sleep, so the worker
# is mid-attempt when each cleanup path fires.
printf 'run %s/claude --ddx-agent-run & %s/codex --ddx-agent-run & sleep 987003\n' "$BIN" "$BIN" \
	> directive.txt

FIRST_BID=""
n=0
while [ "$n" -lt 3 ]; do
	bid="$(ddx bead create "leak scenario $n" --priority 1 --description "do work" --acceptance "done" | tail -1)"
	[ -z "$FIRST_BID" ] && FIRST_BID="$bid"
	n=$((n + 1))
done
git add -A
git commit -qm "scenario fixture"

# desired.json drives the server's startup reconcile to launch one managed
# worker running the script harness against the directive above. restart is
# disabled so a stopped/reaped worker is not auto-replaced mid-assertion.
cat > .ddx/workers/desired.json <<JSON
{"version":1,"desired_count":1,"default_spec":{"mode":"watch","idle_interval":"5s","harness":"script","model":"$REPO/directive.txt"},"restart":{"enabled":false}}
JSON

############################ Phase A: explicit + double stop ###################
log "PHASE A: explicit stop and double stop"
write_config 1h
if ! start_server /work/serverA.log; then fail "phaseA" "server failed to start"; else
	if wait_fakes; then
		WID="$(current_worker_id)"
		log "worker=$WID pid=$(worker_pid)"
		before_stop="$(count_bead_stopped)"
		ddx worker stop "$WID" >/dev/null 2>&1
		assert_no_leak "explicit-stop"
		after_stop="$(count_bead_stopped)"
		if [ "$after_stop" -gt "$before_stop" ]; then
			ok "explicit-stop" "stop emitted a bead.stopped event ($before_stop -> $after_stop)"
		else
			fail "explicit-stop" "expected a bead.stopped event ($before_stop -> $after_stop)"
		fi
		# AC: second stop is a no-op — no duplicate event, no unrelated kill.
		ddx worker stop "$WID" >/dev/null 2>&1
		after_double="$(count_bead_stopped)"
		if [ "$after_double" = "$after_stop" ]; then
			ok "double-stop" "no duplicate bead.stopped event ($after_stop == $after_double)"
		else
			fail "double-stop" "double stop changed bead.stopped count ($after_stop -> $after_double)"
		fi
		if kill -0 "$SRV" 2>/dev/null; then
			ok "double-stop" "server still alive after second stop (no unrelated kill)"
		else
			fail "double-stop" "server died on second stop"
		fi
		assert_no_leak "after-double-stop"
	else
		fail "phaseA" "managed worker / fake agents never started"; cat /work/serverA.log
	fi
fi
stop_server

############################ Phase B: watchdog reap ###########################
log "PHASE B: watchdog reap"
write_config 2s
if ! start_server /work/serverB.log; then fail "phaseB" "server failed to start"; else
	if wait_fakes; then
		WID="$(current_worker_id)"
		WPID="$(worker_pid)"
		log "worker=$WID pid=$WPID"
		# Simulate the run-state a real harness attempt writes (the script harness
		# omits it). Matched to the worker PID, this lets the server watchdog see an
		# in-flight attempt and reap the stalled external worker.
		cat > "$REPO/.ddx/run-state/att-scenario.json" <<JSON
{"bead_id":"$FIRST_BID","attempt_id":"att-scenario","harness":"script","pid":$WPID,"started_at":"2026-01-01T00:00:00Z","refreshed_at":"2026-01-01T00:00:00Z","expires_at":"2031-01-01T00:00:00Z","worktree_path":"$REPO"}
JSON
		reaped=no
		i=0
		while [ "$i" -lt 100 ]; do
			st="$(ddx jq -r '.state' "$REPO/.ddx/workers/$WID/status.json" 2>/dev/null)"
			[ "$st" = "reaped" ] && { reaped=yes; break; }
			pgrep -f "$MARKER" >/dev/null 2>&1 || break
			i=$((i + 5)); sleep 5
		done
		if [ "$reaped" = yes ]; then
			ok "watchdog-reap" "worker state flipped to reaped after ~${i}s"
		else
			fail "watchdog-reap" "worker was not reaped (state=$(ddx jq -r '.state' "$REPO/.ddx/workers/$WID/status.json" 2>/dev/null))"
		fi
		assert_no_leak "watchdog-reap"
	else
		fail "phaseB" "managed worker / fake agents never started"; cat /work/serverB.log
	fi
fi
stop_server

############################ Phase C: server shutdown #########################
log "PHASE C: server shutdown"
write_config 1h
if ! start_server /work/serverC.log; then fail "phaseC" "server failed to start"; else
	if wait_fakes; then
		log "worker=$(current_worker_id) pid=$(worker_pid)"
		# Graceful signal-driven shutdown must tear down the managed worker tree.
		stop_server
		assert_no_leak "server-shutdown"
	else
		fail "phaseC" "managed worker / fake agents never started"; cat /work/serverC.log
	fi
fi
stop_server

############################ verdict ##########################################
if [ "$FAILED" -eq 0 ]; then
	echo "SCENARIO PASS: server-managed worker cleanup left no process leaks"
	exit 0
fi
echo "SCENARIO FAIL: process leak detected"
exit 1
