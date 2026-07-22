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
                clone)
                    _arguments \
                        '--depth[Create a shallow clone with the given history depth]:depth:' \
                        '(-h --help)'{-h,--help}'[Show help]' \
                        && ret=0
                    ;;
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
                switch|branch|checkout-pr|push|pull|doctor|delete|empty)
                    _arguments \
                        '(-h --help)'{-h,--help}'[Show help]' \
                        && ret=0
                    ;;
                tree)
                    _arguments \
                        '--since[Limit to commits since <expr> (empty for all history)]:since:' \
                        '(-a --all)'{-a,--all}'[Show the full history]' \
                        '(-f --follow)'{-f,--follow}'[Follow mode: refresh every N seconds]' \
                        '(-h --help)'{-h,--help}'[Show help]' \
                        && ret=0
                    ;;
                diff)
                    _arguments \
                        '(-m --mode)'{-m,--mode}'[Lock to a diff level: work, index, untracked, head]:mode:(work index untracked head)' \
                        '(-f --follow)'{-f,--follow}'[Follow mode: refresh every N seconds]' \
                        '(-h --help)'{-h,--help}'[Show help]' \
                        && ret=0
                    ;;
                status)
                    _arguments \
                        '1:sections:(branch remote worktree changes head)' \
                        '--flat[Flat porcelain list instead of tree]' \
                        '(-f --follow)'{-f,--follow}'[Follow mode: refresh every N seconds]' \
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
        'clone:Clone a repository, applying gg doctor'\''s naming rules'
        'switch:Switch to a branch interactively'
        'branch:Create a new branch off an existing one and switch to it'
        'checkout-pr:Checkout a pull request from a remote repository'
        'completion:Output shell completion script'
        'status:Show the status of the current git repository'
        'push:Push the current branch to a remote repository'
        "pull:Fetch and integrate the current branch's upstream"
        'tree:Print a colored commit graph across all branches'
        'diff:Show working-tree diff with --compact-summary'
        "doctor:Diagnose known inconsistencies in the repo's remote/worktree layout"
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
