#!/bin/sh
# Targeted pre-commit Go tests for staged packages.
#
# When internal/bead collection-lock / WriteAll surface changes, also run
# internal/exec and a short cmd filter that previously deadlocked on nested
# WriteAll-inside-WithLock (ddx-2a319f04 / ddx-79148c01 CI cascade).
set -u

if [ "$#" -eq 0 ]; then
  exit 0
fi

packages=$(
  printf '%s\n' "$@" |
    xargs -n1 dirname |
    sort -u |
    sed 's|^\./||' |
    sed 's|^cli/||' |
    grep -v '/testdata/' || true
)
if [ -z "${packages:-}" ]; then
  exit 0
fi

# True when the staged package list includes exactly path $1 as a whole line.
has_pkg() {
  printf '%s\n' "$packages" | grep -qx "$1"
}

# Expand bead package changes to dependents that nest collection locks.
expanded="$packages"
if has_pkg "internal/bead"; then
  expanded=$(printf '%s\n%s\n%s\n' $packages internal/exec cmd | sort -u)
fi

status=0
for pkg in $expanded; do
  [ -d "$pkg" ] || continue
  if ls "$pkg"/*_test.go >/dev/null 2>&1; then
    # cmd is huge; when pulled in only as a bead-dependent, run the acceptance
    # tests that hit exec/metric collection locks rather than the full package.
    if [ "$pkg" = "cmd" ] && ! has_pkg "cmd"; then
      if ! go test -short -race -timeout 10m "./$pkg" -run 'TestExec|TestMetricCommands'; then
        status=1
      fi
      continue
    fi
    if ! go test -short -race -timeout 30m "./$pkg"; then
      status=1
    fi
  fi
done

exit "$status"
