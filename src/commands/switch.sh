#!/usr/bin/env -S bash -c 'printf "This file should be sourced, not executed\n"; exit 1'
# gitgum switch command
# Written by Marcin Konowalczyk @lczyk

if [[ -z "${__GITGUM_CMD_SWITCH__:-}" ]]; then

HELP_SWITCH="""
Usage: gitgum switch [branch]
Switch to a branch interactively.
If the branch does not exist, it will ask whether to create it.
If the branch exists, it will switch to it.
If the branch is a remote branch, it will ask whether to create a local branch tracking the remote branch.
"""

function gitgum::cmd::switch() {
    gitgum::check_in_git_repo || return 1
    local _parse_flags_help=$HELP_SWITCH
    _gitgum_parse_flags "$@"
    case $? in 10) return 0 ;; 1) return 1 ;; esac

    # we have a couple of options here:
    # 1. switch to an existing local branch
    # 2. switch to an existing remote branch and create a local tracking branch
    # 3. create a new branch (local)

    local _mode_local="Switch to an existing local branch"
    local _mode_remote="Switch to an existing remote branch and create a local tracking branch"
    local _mode_new="Create a new branch (local)"

    local answer=$(_choose "What do you want to do?" "$_mode_local" "$_mode_remote" "$_mode_new")

    if [[ -z "$answer" ]]; then
        echo "No action selected. Aborting switch."
        return 1
    fi

    if [[ "$answer" == "$_mode_local" ]]; then
        # Switch to an existing local branch
        _gitgum_switch_local
    elif [[ "$answer" == "$_mode_remote" ]]; then
        # Switch to an existing remote branch and create a local tracking branch
        _gitgum_switch_remote
    elif [[ "$answer" == "$_mode_new" ]]; then
        # Create a new branch (local)
        _gitgum_switch_new
    else
        echo "Unknown option: $answer"
        return 1
    fi
}

function _gitgum_switch_local() {
    # find all local branches
    local branches=$(gitgum::local_branches)
    if [[ -z "$branches" ]]; then
        echo "No local branches found. Aborting switch."
        return 1
    fi

    # choose a branch to switch to
    local branch=$(_choose "Select a branch to switch to" $branches)
    if [[ -z "$branch" ]]; then
        echo "No branch selected. Aborting switch."
        return 1
    fi

    # check if the branch is already checked out in another worktree
    local worktree_grep=$(git worktree list | grep "$branch" | awk '{print $1}')
    if [[ -n "$worktree_grep" ]]; then
        echo "Branch '$branch' is already checked out in another worktree: $worktree_grep"
        return 1
    fi

    # switch to the branch
    if ! _maybe_dry_run "git checkout --quiet \"$branch\""; then
        echo "Error: Could not switch to branch '$branch'."
        return 1
    fi
    _echo_if_not_dry_run "Switched to branch '$branch'."
}

function _gitgum_switch_remote() {
    # first we need to find all remote branches
    # then we need to filter out the branches that are already tracked locally

    local remotes=$(git remote -v | awk '{print $1}' | sort -u)
    if [[ -z "$remotes" ]]; then
        echo "No remotes found. Aborting switch."
        return 1
    fi

    # select a remote
    local remote=$(_choose "Select a remote" $remotes)
    if [[ -z "$remote" ]]; then
        echo "No remote selected. Aborting switch."
        return 1
    fi

    # get the remote branches from the selected remote
    local remote_branches=$(
        git branch -r |
            grep "$remote/" |
            grep -v "HEAD ->" |   # filter out the HEAD reference
            sed "s|$remote/||g" | # remove the remote prefix
            sort -u
    )
    if [[ -z "$remote_branches" ]]; then
        echo "No remote branches found for remote '$remote'. Aborting switch."
        return 1
    fi

    # choose a remote branch to switch to
    local remote_branch=$(_choose "Select a remote branch to switch to" $remote_branches)
    if [[ -z "$remote_branch" ]]; then
        echo "No remote branch selected. Aborting switch."
        return 1
    fi

    # check if the branch is already tracked locally
    local local_branch=$(git branch --list "$remote_branch" --format="%(refname:short)")
    if [[ -n "$local_branch" ]]; then
        # the branch is already tracked locally
        echo "Branch '$remote_branch' is already tracked locally as '$local_branch'."
        local tracking_ref=$(git config branch."$local_branch".remote)
        echo "Tracking reference for local branch '$local_branch': '$tracking_ref'"
        if [[ "$tracking_ref" != "$remote" ]]; then
            echo "Local branch '$local_branch' is not tracking remote branch '$remote/$remote_branch'."
            if _confirm "Do you want to set '$remote/$remote_branch' as the tracking reference for local branch '$local_branch'?" "no"; then
                _maybe_dry_run "git branch --set-upstream-to=\"$remote/$remote_branch\" \"$local_branch\""
                _echo_if_not_dry_run "Set tracking reference for local branch '$local_branch' to remote branch '$remote/$remote_branch'."
            else
                echo "Not setting tracking reference. Aborting switch."
                return 1
            fi
            # switch to the local branch
            _maybe_dry_run "git checkout --quiet \"$local_branch\""
            if [[ $? -ne 0 ]]; then
                echo "Error: Could not switch to local branch '$local_branch'."
                return 1
            fi
            _echo_if_not_dry_run "Switched to branch '$local_branch'."

        else
            # the local branch is already tracking the remote branch. just switch to it
            _maybe_dry_run "git checkout --quiet \"$local_branch\""
            if [[ $? -ne 0 ]]; then
                echo "Error: Could not switch to local branch '$local_branch'."
                return 1
            fi
            _echo_if_not_dry_run "Switched to branch '$local_branch' tracking remote branch '$remote/$remote_branch'."
        fi

        # check if the local branch is up to date with the remote branch
        local local_commit=$(git rev-parse "$local_branch")
        local remote_commit
        if ! remote_commit=$(git rev-parse "$remote/$remote_branch" 2>/dev/null); then
            echo "Error: Could not find remote branch '$remote/$remote_branch'."
            return 1
        fi
        if [[ "$local_commit" == "$remote_commit" ]]; then
            echo "Local branch '$local_branch' is up to date with remote branch '$remote/$remote_branch'."
            return 0
        else
            if _confirm "Local branch '$local_branch' is not up to date with remote branch '$remote/$remote_branch'. Do you want to reset the local branch to the remote branch?" "no"; then
                # reset the local branch to the remote branch
                _maybe_dry_run "git reset --hard \"$remote/$remote_branch\""
                _echo_if_not_dry_run "Reset local branch '$local_branch' to remote branch '$remote/$remote_branch'."
            else
                echo "Not resetting local branch. Aborting switch."
                return 1
            fi
        fi

        return 0
    fi

    # the branch is not tracked locally, ask whether to create a local tracking branch
    if _confirm "Branch '$remote_branch' is not tracked locally. Do you want to create a local tracking branch?" "yes"; then
        # create a local tracking branch
        _maybe_dry_run "git checkout -b \"$remote_branch\" \"$remote/$remote_branch\""
        _echo_if_not_dry_run "Created and switched to local branch '$remote_branch' tracking remote branch '$remote/$remote_branch'."
    else
        echo "Not creating a local tracking branch. Aborting switch."
        return 1
    fi
    return 0
}

function _gitgum_switch_new() {
    _not_implemented_error
}

export __GITGUM_CMD_SWITCH__=1
fi
