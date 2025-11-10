#!/usr/bin/env -S bash -c 'printf "This file should be sourced, not executed\n"; exit 1'
# gitgum status command
# Written by Marcin Konowalczyk @lczyk

if [[ -z "${__GITGUM_CMD_STATUS__:-}" ]]; then

HELP_STATUS="""
Usage: gitgum status
This command shows the status of the current git repository as well as the current branch and the state of the remote branches.
"""

function gitgum_cmd_status() {
    local _parse_flags_help=$HELP_STATUS
    _gitgum_parse_flags "$@"
    case $? in 10) return 0 ;; 1) return 1 ;; esac
    # dry run does nothing here

    # Show the remote branches and their status
    _blue "--- BRANCHES ---------------------------"
    git --no-pager branch -vv

    remotes=$(git remote -v | awk '{print($1,$2)}' | sort -u)
    if [[ -z "$remotes" ]]; then
        :
    else
        _blue "--- REMOTES ----------------------------"
        echo "$remotes"
    fi

    # Check whether there are any changes in the working directory
    local changes=$(git status --short)
    if [[ -z "$changes" ]]; then
        :
    else
        _blue "--- CHANGES ----------------------------"
        echo "$changes"
    fi

    # Show the status of the repository at the very end
    _blue "--- STATUS -----------------------------"
    if command -v unbuffer &>/dev/null; then
        # use unbuffer to preserve the color output
        unbuffer git status --short --branch | head -n1
    else
        # if unbuffer is not available, just show the first line of the status
        git status --short --branch | head -n1
    fi
}

export __GITGUM_CMD_STATUS__=1
fi
