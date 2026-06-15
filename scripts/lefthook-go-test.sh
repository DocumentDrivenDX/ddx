#!/bin/sh
set -u

if [ "$#" -eq 0 ]; then
  exit 0
fi

packages=$(
  printf '%s\n' "$@" |
    xargs -n1 dirname |
    sort -u |
    sed 's|^\./||' |
    sed 's|^cli/||'
)

status=0
for pkg in $packages; do
  [ -d "$pkg" ] || continue
  if ls "$pkg"/*_test.go >/dev/null 2>&1; then
    if ! go test -short -race -timeout 30m "./$pkg"; then
      status=1
    fi
  fi
done

exit "$status"
