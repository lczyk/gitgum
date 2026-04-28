_ff_completion() {
    local cur prev opts
    COMPREPLY=()
    if [[ "${BASH_VERSINFO[0]}" -ge 4 ]]; then
        cur="$2"
    else
        cur="${COMP_WORDS[COMP_CWORD]}"
    fi
    prev="$3"

    opts="-m --multi -q --query -p --prompt --header -1 --select-1 --fast --reverse --height --completion -v --version -h --help"

    case "${prev}" in
        --completion)
            COMPREPLY=( $(compgen -W "bash fish zsh nu" -- "${cur}") )
            return 0
            ;;
        -q|--query|-p|--prompt|--header|--height)
            COMPREPLY=()
            return 0
            ;;
    esac

    COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
    return 0
}

if [[ "${BASH_VERSINFO[0]}" -eq 4 && "${BASH_VERSINFO[1]}" -ge 4 || "${BASH_VERSINFO[0]}" -gt 4 ]]; then
    complete -F _ff_completion -o nosort -o bashdefault -o default __GITGUM_CMD__
else
    complete -F _ff_completion -o bashdefault -o default __GITGUM_CMD__
fi
