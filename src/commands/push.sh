#!/usr/bin/env -S bash -c 'printf "This file should be sourced, not executed\n"; exit 1'
# gitgum push command
# Written by Marcin Konowalczyk @lczyk

if [[ -z "${__GITGUM_CMD_PUSH__:-}" ]]; then

HELP_PUSH="""
Usage: gitgum push [options]
Options:
  -h, --help      Show this help message
  -n, --dry-run   Perform a dry run without making any changes
This command allows you to push the current branch to a remote repository.
"""

function gitgum::cmd::push() {
    gitgum::check_in_git_repo || return 1
    local _parse_flags_help=$HELP_PUSH
    _gitgum_parse_flags "$@"
    case $? in 10) return 0 ;; 1) return 1 ;; esac

    # Check if this branch already has a remote tracking branch
    local remote_branch=$(git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null)
    if [[ -n "$remote_branch" ]]; then
        # The current branch already has a remote tracking branch
        echo "Current branch already has a remote tracking branch: $remote_branch"
        if _confirm "Do you want to push to the remote tracking branch?" "yes"; then
            _maybe_dry_run "git push"
            _echo_if_not_dry_run "Pushed to remote tracking branch '$remote_branch'."
            return 0
        else
            echo "Not pushing to remote tracking branch"
        fi
    fi

    local remotes=$(git remote -v | awk '{print $1}' | sort -u)
    local this_branch=$(git rev-parse --abbrev-ref HEAD)
    local remote=$(_choose "Push '$this_branch' to" $remotes)
    if [[ -z "$remote" ]]; then
        echo "No remote selected. Aborting push."
        return 1
    fi
    
    local expected_remote_branch_name="$remote/$this_branch"
    if git ls-remote --exit-code --heads "$remote" "$this_branch" &>/dev/null; then
        # The remote branch already exists
        # check if there are any changes to push
        local local_commit=$(git rev-parse "$this_branch")
        local remote_commit
        if ! remote_commit=$(git rev-parse "$expected_remote_branch_name" 2>/dev/null); then
            echo "Error: Could not find remote branch '$expected_remote_branch_name'."
            return 1
        fi
        if [[ "$local_commit" == "$remote_commit" ]]; then
            echo "No changes to push. Local branch '$this_branch' is up to date with remote branch '$expected_remote_branch_name'."
            return 0
        fi
        _confirm "Remote branch '$expected_remote_branch_name' already exists. Do you want to push to it?" "yes" || return 1
        _maybe_dry_run "git push \"$remote\" \"$this_branch\""
    else
        _confirm "No remote branch '$expected_remote_branch_name' found. Do you want to create it?" || return 1
        _maybe_dry_run "git push -u \"$remote\" \"$this_branch\""
        echo "Created and set tracking reference for '$this_branch' to '$expected_remote_branch_name'."
    fi
}

export __GITGUM_CMD_PUSH__=1
fi
