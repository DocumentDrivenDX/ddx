#!/bin/sh
set -eu

repo_root=${DDX_GIT_CONFIG_HEALTH_ROOT:-$(pwd -P)}
config_file="$repo_root/.git/config"

# Linked worktrees have a .git file. Their per-worktree config is legitimate
# when extensions.worktreeConfig is enabled, so this guard only checks a
# primary checkout's local config file.
if [ ! -f "$config_file" ]; then
	exit 0
fi

worktree_val=$(git config --file "$config_file" --get core.worktree || true)
if [ -z "$worktree_val" ]; then
	exit 0
fi

case "$worktree_val" in
	/*)
		if [ -d "$worktree_val" ]; then
			resolved=$(cd "$worktree_val" && pwd -P)
		else
			resolved=$worktree_val
		fi
		;;
	*)
		if [ -d "$repo_root/.git/$worktree_val" ]; then
			resolved=$(cd "$repo_root/.git/$worktree_val" && pwd -P)
		else
			resolved=$repo_root/.git/$worktree_val
		fi
		;;
esac

if [ "$resolved" != "$repo_root" ]; then
	{
		echo "Invalid local git config: core.worktree=$worktree_val"
		echo "Expected worktree: $repo_root"
		echo "Resolved worktree: $resolved"
		echo "Run: git config --unset core.worktree"
	} >&2
	exit 1
fi
