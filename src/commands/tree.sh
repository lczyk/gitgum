#!/usr/bin/env -S bash -c 'printf "This file should be sourced, not executed\n"; exit 1'
# gitgum tree command
# Written by Marcin Konowalczyk @lczyk

if [[ -z "${__GITGUM_CMD_TREE__:-}" ]]; then

HELP_TREE="""
Usage: gitgum tree
This command shows the git tree structure of the current repository.
"""

function gitgum::cmd::tree() {
    gitgum::check_in_git_repo || return 1
    local _parse_flags_help=$HELP_TREE
    _gitgum_parse_flags "$@"
    case $? in 10) return 0 ;; 1) return 1 ;; esac
    
    git log --graph --oneline --decorate
}

export __GITGUM_CMD_TREE__=1
fi
