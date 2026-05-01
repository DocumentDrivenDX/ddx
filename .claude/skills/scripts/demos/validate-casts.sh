#!/usr/bin/env bash
# Validate all .cast files are parseable asciinema v2 format
set -e

DEMOS_DIR="${1:-website/static/demos}"
errors=0

for cast in "$DEMOS_DIR"/*.cast; do
  name=$(basename "$cast")

  # Check header line is valid JSON with version field
  header=$(head -1 "$cast")
  if ! echo "$header" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d.get('version')==2, 'not v2'" 2>/dev/null; then
    echo "FAIL: $name — invalid header (not asciinema v2)"
    errors=$((errors + 1))
    continue
  fi

  # Check all subsequent lines are valid JSON arrays [time, type, data]
  lineno=1
  while IFS= read -r line; do
    lineno=$((lineno + 1))
    if [ -z "$line" ]; then continue; fi
    if ! echo "$line" | python3 -c "import json,sys; d=json.load(sys.stdin); assert isinstance(d,list) and len(d)>=3" 2>/dev/null; then
      echo "FAIL: $name line $lineno — invalid event"
      errors=$((errors + 1))
      break
    fi
  done < <(tail -n +2 "$cast")

  # Check no local path leaks
  if grep -q "/home/erik" "$cast" 2>/dev/null; then
    echo "WARN: $name — contains /home/erik paths (local environment leak)"
  fi

  # Check no git template warning
  if grep -q "templates not found" "$cast" 2>/dev/null; then
    echo "WARN: $name — contains git template warning"
  fi

  echo "OK: $name"
done

if [ $errors -gt 0 ]; then
  echo ""
  echo "FAILED: $errors cast(s) invalid"
  exit 1
fi
echo ""
echo "All casts valid"
