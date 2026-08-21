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

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
module_root="$repo_root/cli"
cwd_root=$(pwd)

packages_file=$(mktemp)
trap 'rm -f "$packages_file"' EXIT HUP INT TERM

status=0
for input in "$@"; do
  normalized=${input#./}
  resolved=""
  for candidate in "$cwd_root/$normalized" "$repo_root/$normalized" "$module_root/$normalized"; do
    if [ -e "$candidate" ]; then
      resolved=$candidate
      break
    fi
  done
  if [ -z "$resolved" ]; then
    printf 'lefthook-go-test: cannot map Go path %s to a package under %s\n' "$input" "$module_root" >&2
    status=1
    continue
  fi

  case "$resolved" in
    "$module_root"/*)
      rel=${resolved#"$module_root"/}
      pkg=$(dirname "$rel")
      case "$pkg" in
        */testdata|*/testdata/*)
          continue
          ;;
      esac
      printf '%s\n' "$pkg" >>"$packages_file"
      ;;
    *)
      printf 'lefthook-go-test: cannot map Go path %s to a package under %s\n' "$input" "$module_root" >&2
      status=1
      ;;
  esac
done

if [ "$status" -ne 0 ]; then
  exit "$status"
fi

packages=$(sort -u "$packages_file")
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
  [ -d "$module_root/$pkg" ] || continue
  if ls "$module_root/$pkg"/*_test.go >/dev/null 2>&1; then
    # cmd is huge; when pulled in only as a bead-dependent, run the acceptance
    # tests that hit exec/metric collection locks rather than the full package.
    if [ "$pkg" = "cmd" ] && ! has_pkg "cmd"; then
      if ! (cd "$module_root" && go test -short -race -timeout 10m "./$pkg" -run 'TestExec|TestMetricCommands'); then
        status=1
      fi
      continue
    fi
    if ! (cd "$module_root" && go test -short -race -timeout 30m "./$pkg"); then
      status=1
    fi
  fi
done

exit "$status"
