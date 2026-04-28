_gitgum_completion() {
    local i cur prev opts cmd
    COMPREPLY=()
    if [[ "${BASH_VERSINFO[0]}" -ge 4 ]]; then
        cur="$2"
    else
        cur="${COMP_WORDS[COMP_CWORD]}"
    fi
    prev="$3"
    cmd=""

    for i in "${COMP_WORDS[@]:0:COMP_CWORD}"; do
        case "${cmd},${i}" in
            ",$1")
                cmd="gitgum"
                ;;
            gitgum,switch)
                cmd="gitgum_switch"
                ;;
            gitgum,checkout-pr)
                cmd="gitgum_checkout_pr"
                ;;
            gitgum,completion)
                cmd="gitgum_completion"
                ;;
            gitgum,status)
                cmd="gitgum_status"
                ;;
            gitgum,push)
                cmd="gitgum_push"
                ;;
            gitgum,clean)
                cmd="gitgum_clean"
                ;;
            gitgum,delete)
                cmd="gitgum_delete"
                ;;
            gitgum,replay-list)
                cmd="gitgum_replay_list"
                ;;
            gitgum,empty)
                cmd="gitgum_empty"
                ;;
            gitgum,release)
                cmd="gitgum_release"
                ;;
        esac
    done

    case "${cmd}" in
        gitgum)
            opts="-h --help -v --version switch checkout-pr completion status push clean delete replay-list empty release"
            COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
            return 0
            ;;
        gitgum_switch|gitgum_checkout_pr|gitgum_status|gitgum_push|gitgum_delete|gitgum_empty)
            opts="-h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
            return 0
            ;;
        gitgum_completion)
            opts="bash fish zsh nu -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
            return 0
            ;;
        gitgum_clean)
            opts="--changes --untracked --ignored --all -y --yes -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
            return 0
            ;;
        gitgum_replay_list)
            local branches
            branches="$(git for-each-ref --format='%(refname:short)' refs/heads refs/remotes 2>/dev/null)"
            COMPREPLY=( $(compgen -W "${branches}" -- "${cur}") )
            return 0
            ;;
        gitgum_release)
            opts="patch minor major -h --help"
            COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
            return 0
            ;;
    esac
}

if [[ "${BASH_VERSINFO[0]}" -eq 4 && "${BASH_VERSINFO[1]}" -ge 4 || "${BASH_VERSINFO[0]}" -gt 4 ]]; then
    complete -F _gitgum_completion -o nosort -o bashdefault -o default __GITGUM_CMD__
else
    complete -F _gitgum_completion -o bashdefault -o default __GITGUM_CMD__
fi
