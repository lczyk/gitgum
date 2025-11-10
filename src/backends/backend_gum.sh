#!/usr/bin/env -S bash -c 'printf "This file should be sourced, not executed\n"; exit 1'
# Gum backend for gitgum
# Written by Marcin Konowalczyk @lczyk

if [[ -z "${__GITGUM_BACKEND_GUM__:-}" ]]; then

# Choose from a list of options using gum
# Usage: _choose "header text" option1 option2 ...
_choose() {
    local header=$1; shift;
    gum choose --header "$header" "$@"
}

# Confirm a prompt using gum
# Usage: _confirm "prompt text" [default]
# Default can be "yes" or "no", defaults to "yes"
_confirm() {
    local prompt=$1; shift;
    local default="${1:-yes}"
    gum confirm "$prompt" --default="$default"
}

export __GITGUM_BACKEND_GUM__=1
fi
