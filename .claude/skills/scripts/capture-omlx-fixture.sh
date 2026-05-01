#!/usr/bin/env bash
# capture-omlx-fixture.sh — regenerate a testdata/omlx-wire fixture from a
# live omlx endpoint. Wraps `ddx agent run` with AGENT_DEBUG_WIRE_STREAM_FULL=1
# (shipped in ddx-agent v0.3.14 — bead agent-f237e07b) and reshapes the wire
# dump into the fixture JSONL format the consumer-side smoke test expects.
#
# Usage:
#   scripts/capture-omlx-fixture.sh \
#     --endpoint http://vidar:1235/v1 \
#     --model    Qwen3.6-35B-A3B-4bit \
#     --prompt   "Reply with the word ready." \
#     --out      cli/internal/agent/testdata/omlx-wire/happy_path.jsonl
#
# Required tools: ddx (v0.6.0-alpha22+), jq.
#
# What the script does:
#   1. Runs an isolated ddx agent invocation against the given omlx endpoint
#      with AGENT_DEBUG_WIRE=1 AGENT_DEBUG_WIRE_STREAM_FULL=1 so the whole
#      SSE stream is captured to a sidecar wire file.
#   2. Extracts every `dir=="response"` body chunk in arrival order and emits
#      one {"kind":"frame","body":...} record per chunk.
#   3. Prepends a {"kind":"meta",...} record with the endpoint, model, prompt,
#      and a user-supplied expected outcome so the replay test knows what to
#      assert.
#
# Fixtures are checked in. Regenerate only when the omlx server changes wire
# behavior (new frame type, header shift, chunk-size change) in a way the
# existing fixtures no longer represent.

set -euo pipefail

endpoint=""
model=""
prompt=""
out=""
name=""
status="success"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --endpoint) endpoint="$2"; shift 2 ;;
    --model)    model="$2"; shift 2 ;;
    --prompt)   prompt="$2"; shift 2 ;;
    --out)      out="$2"; shift 2 ;;
    --name)     name="$2"; shift 2 ;;
    --status)   status="$2"; shift 2 ;;  # success | error
    -h|--help)
      sed -n '2,28p' "$0"
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

for v in endpoint model prompt out; do
  if [[ -z "${!v}" ]]; then
    echo "missing required --$v" >&2
    exit 2
  fi
done

command -v jq >/dev/null || { echo "jq is required" >&2; exit 2; }
command -v ddx >/dev/null || { echo "ddx is required" >&2; exit 2; }

wire_file="$(mktemp -t omlx-wire.XXXXXX.jsonl)"
trap 'rm -f "$wire_file"' EXIT

echo "capturing wire dump from $endpoint → $wire_file" >&2

AGENT_DEBUG_WIRE=1 \
AGENT_DEBUG_WIRE_STREAM_FULL=1 \
AGENT_DEBUG_WIRE_FILE="$wire_file" \
ddx agent run \
  --harness agent \
  --provider vidar-omlx \
  --model "$model" \
  --prompt-text "$prompt" \
  --timeout 120s \
  >/dev/null || {
    echo "ddx agent run failed — fixture capture incomplete" >&2
    echo "wire dump (may still be useful): $wire_file" >&2
    exit 1
  }

[[ -z "$name" ]] && name="$(basename "$out" .jsonl)"

tmp_out="$(mktemp -t omlx-fixture.XXXXXX.jsonl)"
trap 'rm -f "$wire_file" "$tmp_out"' EXIT

# Meta record — expected outcome is a hint for the replay test. Authors may
# need to hand-edit content_contains / error_contains after inspecting the
# captured body.
jq -cn \
  --arg name "$name" \
  --arg desc "Captured $(date -u +%Y-%m-%dT%H:%M:%SZ) from $endpoint model=$model prompt=$prompt" \
  --arg status "$status" \
  '{kind:"meta", name:$name, description:$desc,
    expected:{status:$status, error_must_not_contain:"unexpected end of JSON input"}}' \
  >> "$tmp_out"

# Frame records — every response chunk in capture order. AGENT_DEBUG_WIRE
# with STREAM_FULL=1 emits one {"dir":"response","body":"…"} record per read.
jq -c 'select(.dir=="response" and (.body|type=="string") and (.body|length>0))
       | {kind:"frame", body:.body}' \
  "$wire_file" >> "$tmp_out"

mv "$tmp_out" "$out"
echo "wrote $(wc -l < "$out") records to $out" >&2
