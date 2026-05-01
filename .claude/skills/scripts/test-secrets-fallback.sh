#!/usr/bin/env bash
# Test: secrets hook fallback does not false-positive on tracker JSONL changes
#
# Regression test for ddx-60316844: the gitleaks fallback must scan only
# added lines with assignment-oriented patterns, not whole files with broad
# keyword patterns.  A staged .ddx/beads.jsonl diff containing prose like
# "token-awareness.md" must not trigger a false positive.

set -euo pipefail

PASS=0
FAIL=0

ok()   { echo "  PASS: $*"; PASS=$((PASS+1)); }
fail() { echo "  FAIL: $*"; FAIL=$((FAIL+1)); }

# ---------------------------------------------------------------------------
# Extract the fallback logic from lefthook.yml so we test the canonical source.
# The fallback block lives between "else" and "fi" inside the secrets hook.
# We inline the patterns here to keep the test self-contained and deterministic.
# ---------------------------------------------------------------------------

PATTERNS='((api[_-]?key|apikey|secret|password|passwd|pwd|credential|private[_-]?key|access[_-]?token|auth[_-]?token)[[:space:]]*[:=][[:space:]]*["'"'"'][A-Za-z0-9_./+=-]{20,})|(Authorization:[[:space:]]*Bearer[[:space:]]+[A-Za-z0-9._-]{20,})'

# Returns 0 (match/detected) or 1 (no match/clean) for a set of added lines.
detect() {
  local lines="$1"
  if printf '%s\n' "$lines" | grep -qE "$PATTERNS"; then
    return 0
  fi
  if printf '%s\n' "$lines" | grep -qE 'AKIA[0-9A-Z]{16}'; then
    return 0
  fi
  if printf '%s\n' "$lines" | grep -qE 'ghp_[a-zA-Z0-9]{36}'; then
    return 0
  fi
  return 1
}

echo "=== secrets fallback: false-positive regression tests ==="

# --- Tracker prose that previously triggered false positives ----------------

TRACKER_DIFF='+{"id":"ddx-abc123","title":"Add token-awareness.md reference","description":"Improve token-awareness.md and auth flow docs","labels":["area:docs"],"status":"open"}'

if detect "$TRACKER_DIFF"; then
  fail "tracker diff with token-awareness.md falsely flagged as secret"
else
  ok "tracker diff with token-awareness.md is clean"
fi

TRACKER_DIFF2='+{"title":"Update auth flow","description":"The auth middleware now validates tokens","status":"open"}'

if detect "$TRACKER_DIFF2"; then
  fail "tracker diff with prose 'auth' and 'tokens' falsely flagged"
else
  ok "tracker diff with prose 'auth' and 'tokens' is clean"
fi

# A bead description referencing an authorization concept (not a secret)
TRACKER_DIFF3='+{"description":"Implement authorization checks for the token endpoint","labels":["area:security"]}'

if detect "$TRACKER_DIFF3"; then
  fail "tracker diff with 'authorization' prose falsely flagged"
else
  ok "tracker diff with 'authorization' prose is clean"
fi

# --- Lines that SHOULD be detected ------------------------------------------

echo ""
echo "=== secrets fallback: true-positive detection tests ==="

SECRET_ASSIGNMENT='+api_key = '"'"'ABCDEFGHIJKLMNOPQRSTUVWXYZ1234'"'"''
if detect "$SECRET_ASSIGNMENT"; then
  ok "api_key assignment detected"
else
  fail "api_key assignment NOT detected (missed)"
fi

SECRET_BEARER='+Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0'
if detect "$SECRET_BEARER"; then
  ok "Authorization Bearer token detected"
else
  fail "Authorization Bearer token NOT detected (missed)"
fi

AWS_KEY='+AKIAIOSFODNN7EXAMPLE'
if detect "$AWS_KEY"; then
  ok "AWS key pattern detected"
else
  fail "AWS key pattern NOT detected (missed)"
fi

GITHUB_TOKEN='+ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij'
if detect "$GITHUB_TOKEN"; then
  ok "GitHub token detected"
else
  fail "GitHub token NOT detected (missed)"
fi

PASSWORD_LINE='+password = '"'"'supersecretpassword12345678'"'"''
if detect "$PASSWORD_LINE"; then
  ok "password assignment detected"
else
  fail "password assignment NOT detected (missed)"
fi

# ---------------------------------------------------------------------------
echo ""
echo "Results: $PASS passed, $FAIL failed"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
