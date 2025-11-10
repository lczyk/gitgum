#!/usr/bin/env -S bash -c 'printf "This file should be sourced, not executed\n"; exit 1'
# Shared helpers for gitgum
# Written by Marcin Konowalczyk @lczyk

if [[ -z "${__GITGUM_HELPERS__:-}" ]]; then

# Color definitions
BLUE="\033[0;34m"
BLACK="\033[0;30m"
NC="\033[0m" # No Color

# Print text in blue/black color
function _blue() { echo -e "${BLACK}$1${NC}"; }

# Global dry-run flag (commands will set this)
declare -g dry_run=0

# Execute command or print it in dry-run mode
_maybe_dry_run() {
    if [[ $dry_run -eq 1 ]]; then
        # squash multiple spaces into one
        echo "> $(echo "$1" | tr -s ' ')"
    else
        eval "$1"
    fi
}

# Echo message only when not in dry-run mode
_echo_if_not_dry_run() {
    [[ $dry_run -eq 0 ]] && echo "$1"
}

# Not implemented error
_not_implemented_error() {
    echo "Error: This feature is not implemented yet."
    return 1
}

# Fatal error
_fatal() {
    echo "Error: $1" >&2
    return 1
}

# Parse common flags (--help, --dry-run)
# Usage: _gitgum_parse_flags "$@"
# Sets global dry_run variable
# Returns 10 if help was requested, 1 on error, 0 on success
function _gitgum_parse_flags() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
        -h | --help)
            echo "$_parse_flags_help"
            return 10
            ;;
        -n | --dry-run)
            dry_run=1
            shift
            ;;
        *)
            echo "Unknown option: $1"
            return 1
            ;;
        esac
    done
    # Print the dry run message
    if [[ $dry_run -eq 1 ]]; then
        echo "Dry run mode enabled. No changes will be made."
    fi
}

export __GITGUM_HELPERS__=1
fi
