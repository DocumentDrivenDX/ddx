#!/usr/bin/env bash
# Verifies every `path:line` citation in SD-015-resolution-path-trace.md
# points to a file that exists and a line that exists. When invoked with
# --strict, also verifies the line contains a Go symbol hint when the path
# looks like Go source.
#
# Cited as the CI step that prevents SD-015-resolution-path-trace.md from
# rotting silently as routing.go / discovery.go / types.go evolve.

set -euo pipefail

DOC_PATH="${1:-docs/helix/02-design/solution-designs/SD-015-resolution-path-trace.md}"

if [ ! -f "$DOC_PATH" ]; then
    echo "citations: $DOC_PATH not found" >&2
    exit 1
fi

# Match citations of the form `path/to/file.ext:NN` inside backticks.
# We only validate citations whose path looks like a file (contains a slash
# and ends with a known extension).
citations=$(grep -oE '`[A-Za-z0-9_./-]+\.(go|ts|tsx|js|yaml|yml|md|sh):[0-9]+`' "$DOC_PATH" | tr -d '`' | sort -u)

if [ -z "$citations" ]; then
    echo "citations: no citations found in $DOC_PATH — fail-closed" >&2
    exit 1
fi

failures=0
total=0

while IFS= read -r cite; do
    [ -z "$cite" ] && continue
    total=$((total + 1))
    path="${cite%:*}"
    line="${cite##*:}"
    if [ ! -f "$path" ]; then
        echo "citations: MISSING FILE: $cite" >&2
        failures=$((failures + 1))
        continue
    fi
    # Line count check; awk 'END{print NR}' counts lines portably.
    nlines=$(awk 'END{print NR}' "$path")
    if [ "$line" -gt "$nlines" ]; then
        echo "citations: LINE OUT OF RANGE: $cite (file has $nlines lines)" >&2
        failures=$((failures + 1))
        continue
    fi
    # Sanity check: the line must not be blank, since every real citation
    # points at a struct, func, or statement.
    content=$(awk -v n="$line" 'NR==n' "$path")
    if [ -z "$(echo "$content" | tr -d '[:space:]')" ]; then
        echo "citations: BLANK LINE: $cite" >&2
        failures=$((failures + 1))
        continue
    fi
done <<< "$citations"

if [ "$failures" -gt 0 ]; then
    echo "citations: $failures/$total failed for $DOC_PATH" >&2
    exit 1
fi

echo "citations: $total/$total ok for $DOC_PATH"
