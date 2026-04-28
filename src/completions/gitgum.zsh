#compdef __GITGUM_CMD__

_gitgum() {
    local context curcontext="$curcontext" state line ret=1
    typeset -A opt_args

    _arguments -C \
        '(-h --help)'{-h,--help}'[Show help]' \
        '(-v --version)'{-v,--version}'[Show version]' \
        '1: :_gitgum_commands' \
        '*::arg:->args' \
        && ret=0

    case $state in
        args)
            case $line[1] in
                completion)
                    _arguments \
                        '1:shell:((bash\:"Bourne Again SHell" fish\:"Friendly Interactive SHell" zsh\:"Z shell" nu\:"Nushell"))' \
                        '(-h --help)'{-h,--help}'[Show help]' \
                        && ret=0
                    ;;
                clean)
                    _arguments \
                        '--changes[Discard staged and unstaged changes (default: true)]' \
                        '--untracked[Remove untracked files (default: true)]' \
                        '--ignored[Remove ignored files (default: false)]' \
                        '--all[Enable all cleanup options]' \
                        '(-y --yes)'{-y,--yes}'[Skip confirmation prompt]' \
                        '(-h --help)'{-h,--help}'[Show help]' \
                        && ret=0
                    ;;
                replay-list)
                    _arguments \
                        '1:branch A:_gitgum_branches' \
                        '2:branch B:_gitgum_branches' \
                        && ret=0
                    ;;
                release)
                    _arguments \
                        '1:bump:(patch minor major)' \
                        && ret=0
                    ;;
                switch|checkout-pr|status|push|delete|empty)
                    _arguments \
                        '(-h --help)'{-h,--help}'[Show help]' \
                        && ret=0
                    ;;
            esac
            ;;
    esac

    return ret
}

_gitgum_commands() {
    local -a commands
    commands=(
        'switch:Switch to a branch interactively'
        'checkout-pr:Checkout a pull request from a remote repository'
        'completion:Output shell completion script'
        'status:Show the status of the current git repository'
        'push:Push the current branch to a remote repository'
        'clean:Discard working tree changes and untracked files'
        'delete:Delete a local branch and optionally its remote tracking branch'
        'replay-list:List commits on branch A since divergence from trunk B'
        'empty:Create an empty commit and optionally push it'
        'release:Bump VERSION (or latest tag), commit, and tag'
    )
    _describe -t commands 'command' commands
}

_gitgum_branches() {
    local -a branches
    branches=(${(f)"$(git for-each-ref --format='%(refname:short)' refs/heads refs/remotes 2>/dev/null)"})
    _describe -t branches 'branch' branches
}

if [ "$funcstack[1]" = "_gitgum" ]; then
    _gitgum "$@"
else
    compdef _gitgum __GITGUM_CMD__
fi
