#compdef __GITGUM_CMD__#compdef __GITGUM_CMD__#compdef __GITGUM_CMD__

# Zsh completion for gitgum (template - __GITGUM_CMD__ will be replaced)

# Zsh completion for gitgum (template - __GITGUM_CMD__ will be replaced)# Zsh completion for gitgum (template - __GITGUM_CMD__ will be replaced)

_gitgum() {

    local -a commands

    commands=(

        'switch:Switch to a branch interactively'_gitgum() {_gitgum() {

        'completion:Output shell completion script'

    )    local -a commands    local -a commands



    local -a global_opts    commands=(    commands=(

    global_opts=(

        '-h[Show help message]'        'tree:Show the git tree structure'        'tree:Show the git tree structure'

        '--help[Show help message]'

    )        'push:Push the current branch to a remote repository'        'push:Push the current branch to a remote repository'



    if (( CURRENT == 2 )); then        'delete:Delete a branch or a remote branch'        'delete:Delete a branch or a remote branch'

        _describe 'command' commands

        return        'status:Show the status of the current git repository'        'status:Show the status of the current git repository'

    fi

        'commit:Commit changes in the current branch'        'commit:Commit changes in the current branch'

    local cmd="${words[2]}"

        'switch:Switch to a branch interactively'        'switch:Switch to a branch interactively'

    case "$cmd" in

        completion)        'merge-into:Merge current branch into another branch'        'merge-into:Merge current branch into another branch'

            if (( CURRENT == 3 )); then

                local -a shells        'completion:Output shell completion script'        'completion:Output shell completion script'

                shells=('fish' 'bash' 'zsh')

                _describe 'shell' shells        'help:Show help message'        'help:Show help message'

            else

                _arguments \    )    )

                    '-h[Show help message]' \

                    '--help[Show help message]'

            fi

            ;;    local -a global_opts    local -a global_opts

        *)

            _arguments $global_opts    global_opts=(    global_opts=(

            ;;

    esac        '-h[Show help message]'        '-h[Show help message]'

}

        '--help[Show help message]'        '--help[Show help message]'

_gitgum "$@"

    )    )



    local -a cmd_opts    local -a cmd_opts

    cmd_opts=(    cmd_opts=(

        '-h[Show help message]'        '-h[Show help message]'

        '--help[Show help message]'        '--help[Show help message]'

        '-n[Perform a dry run]'        '-n[Perform a dry run]'

        '--dry-run[Perform a dry run]'        '--dry-run[Perform a dry run]'

    )    )



    if (( CURRENT == 2 )); then    if (( CURRENT == 2 )); then

        _describe 'command' commands        _describe 'command' commands

        return        return

    fi    fi



    local cmd="${words[2]}"    local cmd="${words[2]}"



    case "$cmd" in    case "$cmd" in

        completion)        completion)

            if (( CURRENT == 3 )); then            if (( CURRENT == 3 )); then

                local -a shells                local -a shells

                shells=('fish' 'bash' 'zsh')                shells=('fish' 'bash' 'zsh')

                _describe 'shell' shells                _describe 'shell' shells

            else            else

                _arguments \                _arguments \

                    '-h[Show help message]' \                    '-h[Show help message]' \

                    '--help[Show help message]'                    '--help[Show help message]'

            fi            fi

            ;;            ;;

        tree|status)        tree|status)

            _arguments \            _arguments \

                '-h[Show help message]' \                '-h[Show help message]' \

                '--help[Show help message]'                '--help[Show help message]'

            ;;            ;;

        push|delete|commit|switch|merge-into)        push|delete|commit|switch|merge-into)

            _arguments $cmd_opts            _arguments $cmd_opts

            ;;            ;;

        help)        help)

            ;;            ;;

        *)        *)

            _arguments $global_opts            _arguments $global_opts

            ;;            ;;

    esac    esac

}}



_gitgum "$@"_gitgum "$@"

