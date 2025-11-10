#!/usr/bin/env -S bash -c 'printf "This file should be sourced, not executed\n"; exit 1'
# gitgum completion command
# Written by Marcin Konowalczyk @lczyk

if [[ -z "${__GITGUM_CMD_COMPLETION__:-}" ]]; then

HELP_COMPLETION="""
Usage: gitgum completion <shell>
Arguments:
  shell           Shell type: fish, bash, or zsh
Options:
  -h, --help      Show this help message
This command outputs shell completion scripts that can be sourced to enable
command completion. The completions adapt to the actual command name (e.g., if
gitgum is symlinked as 'gg', the completions will work for 'gg').
Example usage in fish:
  gitgum completion fish | source
"""

function gitgum_cmd_completion() {
    local _parse_flags_help=$HELP_COMPLETION
    
    # Parse arguments (only support --help, no dry-run)
    local kind=""
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -h|--help)
                echo "$HELP_COMPLETION"
                return 0
                ;;
            -*)
                echo "Error: Unknown option '$1'"
                echo "$HELP_COMPLETION"
                return 1
                ;;
            *)
                if [[ -z "$kind" ]]; then
                    kind="$1"
                    shift
                else
                    echo "Error: Unexpected argument '$1'"
                    return 1
                fi
                ;;
        esac
    done

    # Check if kind was provided
    if [[ -z "$kind" ]]; then
        echo "Error: Missing required argument <shell>"
        echo "$HELP_COMPLETION"
        return 1
    fi

    # Validate kind
    case "$kind" in
        fish|bash|zsh)
            ;;
        *)
            echo "Error: Invalid shell type '$kind'. Must be one of: fish, bash, zsh"
            return 1
            ;;
    esac

    # Detect the actual command name (handles symlinks)
    # Use $0 from the parent script context, not realpath
    local cmd_name=$(basename "$0")

    # Get script directory
    local script_dir="$(cd "$(dirname "$(realpath "${BASH_SOURCE[0]}")")" && cd .. && pwd)"
    local completion_file="$script_dir/completions/gitgum.$kind"

    # Check if completion file exists
    if [[ ! -f "$completion_file" ]]; then
        echo "Error: Completion file not found: $completion_file"
        return 1
    fi

    # Output the completion script with placeholder substitution
    sed "s/__GITGUM_CMD__/$cmd_name/g" "$completion_file"
}

export __GITGUM_CMD_COMPLETION__=1
fi
