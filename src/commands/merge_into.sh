#!/usr/bin/env -S bash -c 'printf "This file should be sourced, not executed\n"; exit 1'
# gitgum merge-into command
# Written by Marcin Konowalczyk @lczyk

if [[ -z "${__GITGUM_CMD_MERGE_INTO__:-}" ]]; then

HELP_MERGE_INTO="""
Usage: gitgum merge-into [options]
Options:
  -h, --help      Show this help message
  -n, --dry-run   Perform a dry run without making any changes
This command merges the current branch into another branch using --no-ff,
then returns to the original branch. Equivalent to checking out the target,
merging, and checking out the original branch again.
"""

function gitgum_cmd_merge_into() {
    local _parse_flags_help=$HELP_MERGE_INTO
    _gitgum_parse_flags "$@"
    case $? in 10) return 0 ;; 1) return 1 ;; esac

    # Get the current branch
    local original_branch=$(git rev-parse --abbrev-ref HEAD)
    if [[ -z "$original_branch" ]]; then
        echo "Error: Could not determine current branch."
        return 1
    fi

    # Get all local branches excluding the current one
    local all_branches=$(gitgum_local_branches)
    if [[ -z "$all_branches" ]]; then
        echo "No other branches found to merge into."
        return 1
    fi

    # Filter out the current branch
    local branches=$(echo "$all_branches" | grep -v "^${original_branch}$")
    if [[ -z "$branches" ]]; then
        echo "No other branches found to merge into."
        return 1
    fi

    # Choose target branch
    local target_branch=$(_choose "Merge '$original_branch' into which branch?" $branches)
    if [[ -z "$target_branch" ]]; then
        echo "No branch selected. Aborting merge."
        return 1
    fi

    # Check if target branch is checked out in another worktree
    local worktree_grep=$(git worktree list | grep "$target_branch" | awk '{print $1}')
    if [[ -n "$worktree_grep" ]]; then
        echo "Branch '$target_branch' is already checked out in another worktree: $worktree_grep"
        echo "Cannot merge into a branch that is checked out elsewhere."
        return 1
    fi

    # Confirm the merge
    if ! _confirm "Merge '$original_branch' into '$target_branch' with --no-ff?" "yes"; then
        echo "Merge cancelled."
        return 1
    fi

    # Setup cleanup to always return to original branch
    local merge_status=0
    local cleanup_needed=0

    # Checkout target branch
    if ! _maybe_dry_run "git checkout --quiet \"$target_branch\""; then
        echo "Error: Could not checkout branch '$target_branch'."
        return 1
    fi
    cleanup_needed=1
    _echo_if_not_dry_run "Checked out branch '$target_branch'."

    # Perform the merge
    if ! _maybe_dry_run "git merge --no-ff \"$original_branch\""; then
        echo "Error: Merge failed. You may have conflicts to resolve."
        merge_status=1
    else
        _echo_if_not_dry_run "Successfully merged '$original_branch' into '$target_branch'."
    fi

    # Always return to original branch
    if [[ $cleanup_needed -eq 1 ]]; then
        if ! _maybe_dry_run "git checkout --quiet \"$original_branch\""; then
            echo "Error: Could not return to original branch '$original_branch'."
            echo "You are currently on branch '$target_branch'."
            return 1
        fi
        _echo_if_not_dry_run "Returned to branch '$original_branch'."
    fi

    # If merge failed, exit now
    if [[ $merge_status -ne 0 ]]; then
        return 1
    fi

    # Rebase current branch on top of the target branch
    if _confirm "Rebase '$original_branch' on top of '$target_branch'?" "yes"; then
        if ! _maybe_dry_run "git rebase \"$target_branch\""; then
            echo "Error: Rebase failed. You may need to resolve conflicts."
            echo "After resolving conflicts, run: git rebase --continue"
            echo "Or abort the rebase with: git rebase --abort"
            return 1
        fi
        _echo_if_not_dry_run "Successfully rebased '$original_branch' on top of '$target_branch'."
    fi

    # Ask about pushing the target branch
    if _confirm "Push '$target_branch' to its remote now?" "no"; then
        # Check if target branch has a remote tracking branch
        local remote_branch=$(git rev-parse --abbrev-ref --symbolic-full-name "$target_branch@{u}" 2>/dev/null)
        if [[ -n "$remote_branch" ]]; then
            # Switch to target branch temporarily to push
            _maybe_dry_run "git checkout --quiet \"$target_branch\""
            _maybe_dry_run "git push"
            _maybe_dry_run "git checkout --quiet \"$original_branch\""
            _echo_if_not_dry_run "Pushed '$target_branch' to '$remote_branch'."
        else
            echo "No remote tracking branch found for '$target_branch'."
            echo "You can push it manually later with: git push -u <remote> $target_branch"
        fi
    fi

    return 0
}

export __GITGUM_CMD_MERGE_INTO__=1
fi
