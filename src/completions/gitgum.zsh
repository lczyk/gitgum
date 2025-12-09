#compdef __GITGUM_CMD__

# Zsh completion for gitgum (template - __GITGUM_CMD__ will be replaced)

_gitgum() {
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
    )

    local -a global_opts
    global_opts=(
        '-h[Show help message]'
        '--help[Show help message]'
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
        status)
            _arguments \
                '-h[Show help message]' \
                '--help[Show help message]'
            ;;
        switch)
            _arguments \
                '-h[Show help message]' \
                '--help[Show help message]'
            ;;
        checkout-pr)
            _arguments \
                '-h[Show help message]' \
                '--help[Show help message]'
            ;;
        push)
            _arguments \
                '-h[Show help message]' \
                '--help[Show help message]'
            ;;
        clean)
            _arguments \
                '--changes[Discard staged and unstaged changes (default)]' \
                '--untracked[Remove untracked files (default)]' \
                '--ignored[Remove ignored files]' \
                '--all[Enable all cleanup options]' \
                '--yes[Skip confirmation prompt]' \
                '-y[Skip confirmation prompt]' \
                '-h[Show help message]' \
                '--help[Show help message]'
            ;;
        delete)
            _arguments \
                '-h[Show help message]' \
                '--help[Show help message]'
            ;;
        replay-list)
            _arguments \
                '-h[Show help message]' \
                '--help[Show help message]'
            ;;
        *)
            _arguments $global_opts
            ;;
    esac
}

_gitgum "$@"

