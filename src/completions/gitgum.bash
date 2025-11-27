# Bash completion for gitgum (template - __GITGUM_CMD__ will be replaced)

_gitgum_completion() {
    local cur prev words cword
    _init_completion || return

    local commands="switch checkout-pr completion status push"
    local global_opts="-h --help"

    # Complete first argument (command)
    if [[ $cword -eq 1 ]]; then
        COMPREPLY=($(compgen -W "$commands" -- "$cur"))
        return 0
    fi

    # Get the command
    local cmd="${words[1]}"

    if [[ "$cmd" == "completion" && $cword -eq 2 ]]; then
        COMPREPLY=($(compgen -W "fish bash zsh" -- "$cur"))
        return 0
    fi

    case "$cmd" in
        completion)
            COMPREPLY=($(compgen -W "-h --help" -- "$cur"))
            ;;
        status)
            COMPREPLY=($(compgen -W "-h --help" -- "$cur"))
            ;;
        checkout-pr)
            COMPREPLY=($(compgen -W "-h --help" -- "$cur"))
            ;;
        push)
            COMPREPLY=($(compgen -W "-h --help" -- "$cur"))
            ;;
        switch)
            COMPREPLY=($(compgen -W "-h --help" -- "$cur"))
            ;;
        *)
            COMPREPLY=($(compgen -W "$global_opts" -- "$cur"))
            ;;
    esac

    return 0
}

complete -F _gitgum_completion __GITGUM_CMD__
