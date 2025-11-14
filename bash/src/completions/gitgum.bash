# Bash completion for gitgum (template - __GITGUM_CMD__ will be replaced)

_gitgum_completion() {
    local cur prev words cword
    _init_completion || return

    local commands="tree push delete status commit switch merge-into completion help"
    local global_opts="-h --help"
    local cmd_opts="-h --help -n --dry-run"

    # Complete first argument (command)
    if [[ $cword -eq 1 ]]; then
        COMPREPLY=($(compgen -W "$commands" -- "$cur"))
        return 0
    fi

    # Get the command
    local cmd="${words[1]}"

    # Handle completion command specially
    if [[ "$cmd" == "completion" && $cword -eq 2 ]]; then
        COMPREPLY=($(compgen -W "fish bash zsh" -- "$cur"))
        return 0
    fi

    # Complete options for commands
    case "$cmd" in
        tree|status)
            COMPREPLY=($(compgen -W "-h --help" -- "$cur"))
            ;;
        push|delete|commit|switch|merge-into)
            COMPREPLY=($(compgen -W "$cmd_opts" -- "$cur"))
            ;;
        completion)
            COMPREPLY=($(compgen -W "-h --help" -- "$cur"))
            ;;
        help)
            COMPREPLY=()
            ;;
        *)
            COMPREPLY=($(compgen -W "$global_opts" -- "$cur"))
            ;;
    esac

    return 0
}

complete -F _gitgum_completion __GITGUM_CMD__
