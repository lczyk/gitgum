#!/usr/bin/env -S bash -c 'printf "This file should be sourced, not executed\n"; exit 1'
# gitgum delete command
# Written by Marcin Konowalczyk @lczyk

if [[ -z "${__GITGUM_CMD_DELETE__:-}" ]]; then

HELP_DELETE="""
Usage: gitgum delete
Delete a branch or a remote branch.
If deleting a local branch, it will also ask whether to delete the remote branch if it exists.
"""

function gitgum::cmd::delete() {
    gitgum::check_in_git_repo || return 1
    local _parse_flags_help=$HELP_DELETE
    _gitgum_parse_flags "$@"
    case $? in 10) return 0 ;; 1) return 1 ;; esac

    # find all local branches
    local branches=$(gitgum::local_branches)
    if [[ -z "$branches" ]]; then
        echo "No local branches found."
        return 1
    fi

    # choose a branch to delete
    local branch=$(_choose "Select a branch to delete" $branches)
    if [[ -z "$branch" ]]; then
        echo "No branch selected. Aborting delete."
        return 1
    fi

    # if the branch name is 'main' or 'master', warn the user
    if [[ "$branch" == "main" ]] || [[ "$branch" == "master" ]]; then
        _confirm "You are about to delete the '$branch' branch. This is usually the main branch of the repository. Are you sure you want to proceed?" "no" || return 1
    fi

    # check if the branch is the current branch
    local current_branch=$(git rev-parse --abbrev-ref HEAD)
    if [[ "$branch" == "$current_branch" ]]; then
        _confirm "You are currently on branch '$branch'. Do you want to switch to another branch before deleting it?" "yes" || return 1
        # switch to another branch
        local branches_sans_current=$(echo "$branches" | grep -v "$branch")
        if [[ -z "$branches_sans_current" ]]; then
            echo "No other branches found to switch to. Aborting delete."
            return 1
        fi
        local other_branch=$(_choose "Select a branch to switch to" $branches_sans_current)
        if [[ -z "$other_branch" ]]; then
            echo "No branch selected. Aborting delete."
            return 1
        fi
        _maybe_dry_run "git checkout \"$other_branch\""
        _echo_if_not_dry_run "Switched to branch '$other_branch'."
    fi

    # check whether the branch is tracking a remote branch
    local remote_branch=$(git branch -r | grep "$branch" | sed 's/^[ *]*//')
    local needs_to_delete_remote=-1 # -1 means no remote, 0 means no, 1 means yes
    if [[ -n "$remote_branch" ]]; then
        if _confirm "Branch '$branch' is tracking remote branch '$remote_branch'. Do you want to delete the remote branch as well?" "no"; then
            needs_to_delete_remote=1
        else
            needs_to_delete_remote=0
        fi
    fi

    # delete the local branch
    if ! _maybe_dry_run "git branch -d \"$branch\""; then
        echo "Could not delete branch '$branch'. It may not be fully merged."
        if [[ $needs_to_delete_remote -eq 1 ]]; then
            if _confirm "Branch '$branch' is not fully merged. Do you want to force delete the local branch and the remote branch?" "no"; then
                needs_to_delete_remote=1
            else
                needs_to_delete_remote=0
            fi
        else
            # we do not have a remote branch. for now just check whether to delete the local branch
            if _confirm "Branch '$branch' does not track a remote branch. Do you want to force delete the local branch?" "no"; then
                if ! _maybe_dry_run "git branch -D \"$branch\""; then
                    echo "Error: Could not force delete branch '$branch'."
                    return 1
                fi
            fi
        fi

        exit 1
    fi

    # if we need to delete the remote branch, do it now
    if [[ $needs_to_delete_remote -eq 1 ]]; then
        if ! _maybe_dry_run "git push --delete \
            \"$(echo "$remote_branch" | cut -d'/' -f1)\" \
            \"$(echo "$remote_branch" | cut -d'/' -f2-)\""; then
            echo "Error: Could not delete remote branch '$remote_branch'."
            return 1
        fi
        _echo_if_not_dry_run "Deleted remote branch '$remote_branch'."
    fi
}

export __GITGUM_CMD_DELETE__=1
fi
