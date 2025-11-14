#compdef __GITGUM_CMD__

# Zsh completion for gitgum (template - __GITGUM_CMD__ will be replaced)

_gitgum() {
    local -a commands

    commands=(
        'switch:Switch to a branch interactively'
        'completion:Output shell completion script'
        'status:Show the status of the current git repository'
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
        *)
            _arguments $global_opts
            ;;
    esac
}

