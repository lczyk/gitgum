#compdef __GITGUM_CMD__
# Zsh completion for gitgum (template - __GITGUM_CMD__ will be replaced)

_gitgum() {
    local -a commands
    commands=(
        'tree:Show the git tree structure'
        'push:Push the current branch to a remote repository'
        'delete:Delete a branch or a remote branch'
        'status:Show the status of the current git repository'
        'commit:Commit changes in the current branch'
        'switch:Switch to a branch interactively'
        'merge-into:Merge current branch into another branch'
        'completion:Output shell completion script'
        'help:Show help message'
    )

    local -a global_opts
    global_opts=(
        '-h[Show help message]'
        '--help[Show help message]'
    )

    local -a cmd_opts
    cmd_opts=(
        '-h[Show help message]'
        '--help[Show help message]'
        '-n[Perform a dry run]'
        '--dry-run[Perform a dry run]'
    )

    if (( CURRENT == 2 )); then
        _describe 'command' commands
        return
    fi

    local cmd="${words[2]}"

    case "$cmd" in
        completion)
            if (( CURRENT == 3 )); then
                local -a shells
                shells=('fish' 'bash' 'zsh')
                _describe 'shell' shells
            else
                _arguments \
                    '-h[Show help message]' \
                    '--help[Show help message]'
            fi
            ;;
        tree|status)
            _arguments \
                '-h[Show help message]' \
                '--help[Show help message]'
            ;;
        push|delete|commit|switch|merge-into)
            _arguments $cmd_opts
            ;;
        help)
            ;;
        *)
            _arguments $global_opts
            ;;
    esac
}

_gitgum "$@"
