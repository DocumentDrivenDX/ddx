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

bare_val=$(git config --file "$config_file" --get core.bare || true)
worktree_val=$(git config --file "$config_file" --get core.worktree || true)
hooks_path_val=$(git config --file "$config_file" --get core.hooksPath || true)
user_name_val=$(git config --file "$config_file" --get user.name || true)
user_email_val=$(git config --file "$config_file" --get user.email || true)
failed=0

case "$bare_val" in
	true | TRUE | True | 1 | yes | on)
		{
			echo "Invalid local git config: core.bare=$bare_val"
			echo "This checkout has a working tree and must not be marked bare."
			echo "Run: git config --unset core.bare"
		} >&2
		failed=1
		;;
esac

if [ -n "$worktree_val" ]; then
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
		failed=1
	fi
fi

if [ -n "$hooks_path_val" ]; then
	case "$hooks_path_val" in
		/*)
			resolved_hooks=$hooks_path_val
			;;
		*)
			resolved_hooks=$repo_root/$hooks_path_val
			;;
	esac

	case "$resolved_hooks" in
		"$repo_root/.git/hooks" | "$repo_root/.git/hooks/")
			{
				echo "Invalid local git config: core.hooksPath=$hooks_path_val"
				echo "Local core.hooksPath prevents lefthook from managing its hook path."
				echo "Run: git config --unset-all --local core.hooksPath"
				echo "Then run: lefthook install --reset-hooks-path"
			} >&2
			failed=1
			;;
		*)
			{
				echo "Invalid local git config: core.hooksPath=$hooks_path_val"
				echo "Project-local core.hooksPath is unsupported because it can bypass lefthook-managed hooks."
				echo "Run: git config --unset-all --local core.hooksPath"
				echo "Then run: lefthook install --reset-hooks-path"
			} >&2
			failed=1
			;;
	esac
fi

if [ "$user_name_val" = "DDx Fixture" ] || [ "$user_email_val" = "fixture@ddx.test" ]; then
	{
		echo "Invalid local git config: fixture identity leaked into primary checkout."
		echo "user.name=${user_name_val:-<unset>}"
		echo "user.email=${user_email_val:-<unset>}"
		echo "Run: git config --unset user.name"
		echo "Run: git config --unset user.email"
	} >&2
	failed=1
fi

if [ "$failed" -ne 0 ]; then
	exit 1
fi
