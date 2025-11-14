# Fish completion for gitgum (template - __GITGUM_CMD__ will be replaced)

function __fish_gitgum_needs_command
    set -l cmd (commandline -opc)
    test (count $cmd) -eq 1
end

function __fish_gitgum_using_command
    set -l cmd (commandline -opc)
    test (count $cmd) -gt 1; and contains -- $cmd[2] $argv
end

# Main commands
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a switch -d "Switch to a branch interactively"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a completion -d "Output shell completion script"

# Global options
complete -f -c __GITGUM_CMD__ -s h -l help -d "Show help message"

# Completion command - suggest shell types
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command completion' -a "fish bash zsh" -d "Shell type"
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command completion' -s h -l help -d "Show help for completion"
