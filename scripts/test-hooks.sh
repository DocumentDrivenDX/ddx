#!/usr/bin/env bash
# Smoke tests for lefthook hook logic.
# Run: bash scripts/test-hooks.sh
# Returns non-zero on failure.

set -euo pipefail

PASS=0
FAIL=0

# Pattern extracted from lefthook.yml no-debug hook
NO_DEBUG_PATTERN='(console\.log|console\.debug\(|console\.error\("DEBUG|fmt\.Print|pdb\.set_trace|debugger|breakpoint\(\))'

pass() { echo "PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "FAIL: $1"; FAIL=$((FAIL + 1)); }

# Require rg
if ! command -v rg >/dev/null 2>&1; then
  echo "ERROR: ripgrep (rg) is required to run these tests"
  exit 1
fi

# ---- no-debug hook tests ----

# Should BLOCK: console.log
f=$(mktemp /tmp/test-hooks-XXXXXX.js)
echo 'console.log("hello")' > "$f"
if rg -q "$NO_DEBUG_PATTERN" "$f"; then
  pass "no-debug blocks console.log"
else
  fail "no-debug should block console.log"
fi
rm -f "$f"

# Should BLOCK: console.debug(
f=$(mktemp /tmp/test-hooks-XXXXXX.js)
echo 'console.debug("value")' > "$f"
if rg -q "$NO_DEBUG_PATTERN" "$f"; then
  pass "no-debug blocks console.debug("
else
  fail "no-debug should block console.debug("
fi
rm -f "$f"

# Should BLOCK: console.error("DEBUG ...
f=$(mktemp /tmp/test-hooks-XXXXXX.js)
echo 'console.error("DEBUG something")' > "$f"
if rg -q "$NO_DEBUG_PATTERN" "$f"; then
  pass 'no-debug blocks console.error("DEBUG ...'
else
  fail 'no-debug should block console.error("DEBUG ...'
fi
rm -f "$f"

# Should BLOCK: breakpoint()
f=$(mktemp /tmp/test-hooks-XXXXXX.py)
echo 'breakpoint()' > "$f"
if rg -q "$NO_DEBUG_PATTERN" "$f"; then
  pass "no-debug blocks breakpoint()"
else
  fail "no-debug should block breakpoint()"
fi
rm -f "$f"

# Should BLOCK: pdb.set_trace
f=$(mktemp /tmp/test-hooks-XXXXXX.py)
echo 'pdb.set_trace()' > "$f"
if rg -q "$NO_DEBUG_PATTERN" "$f"; then
  pass "no-debug blocks pdb.set_trace"
else
  fail "no-debug should block pdb.set_trace"
fi
rm -f "$f"

# Should BLOCK: debugger
f=$(mktemp /tmp/test-hooks-XXXXXX.js)
echo 'debugger' > "$f"
if rg -q "$NO_DEBUG_PATTERN" "$f"; then
  pass "no-debug blocks debugger"
else
  fail "no-debug should block debugger"
fi
rm -f "$f"

# Should BLOCK: fmt.Print
f=$(mktemp /tmp/test-hooks-XXXXXX.go)
echo 'fmt.Print("x")' > "$f"
if rg -q "$NO_DEBUG_PATTERN" "$f"; then
  pass "no-debug blocks fmt.Print"
else
  fail "no-debug should block fmt.Print"
fi
rm -f "$f"

# Should PASS: clean file
f=$(mktemp /tmp/test-hooks-XXXXXX.js)
echo 'const x = 1;' > "$f"
if ! rg -q "$NO_DEBUG_PATTERN" "$f"; then
  pass "no-debug passes clean file"
else
  fail "no-debug should pass clean file"
fi
rm -f "$f"

# Should PASS: console.error without DEBUG prefix
f=$(mktemp /tmp/test-hooks-XXXXXX.js)
echo 'console.error("something went wrong")' > "$f"
if ! rg -q "$NO_DEBUG_PATTERN" "$f"; then
  pass "no-debug passes console.error without DEBUG prefix"
else
  fail "no-debug should pass console.error without DEBUG prefix"
fi
rm -f "$f"

# ---- summary ----
echo ""
echo "Results: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
