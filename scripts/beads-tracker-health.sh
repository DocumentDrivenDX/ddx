#!/bin/sh
set -eu

repo_root=${DDX_BEADS_TRACKER_HEALTH_ROOT:-$(pwd -P)}
tracker=".ddx/beads.jsonl"
tracker_path="$repo_root/$tracker"

if ! git -C "$repo_root" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	exit 0
fi

if ! git -C "$repo_root" ls-files --error-unmatch -- "$tracker" >/dev/null 2>&1; then
	if [ -e "$tracker_path" ]; then
		{
			echo "$tracker exists but is not tracked"
			echo "Run: git add $tracker"
		} >&2
		exit 1
	fi
	exit 0
fi

if git -C "$repo_root" diff --quiet -- "$tracker"; then
	exit 0
fi

{
	echo "$tracker has unstaged changes"
	if ! git -C "$repo_root" diff --cached --quiet -- "$tracker"; then
		echo "The tracker is partially staged; stage the complete tracker state before committing."
	fi
	echo "Run: git add $tracker"
} >&2
exit 1
