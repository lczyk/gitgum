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
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a checkout-pr -d "Checkout a pull request from a remote repository"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a completion -d "Output shell completion script"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a status -d "Show the status of the current git repository"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a push -d "Push the current branch to a remote repository"

# Global options
complete -f -c __GITGUM_CMD__ -s h -l help -d "Show help message"

# Completion command - suggest shell types
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command completion' -a "fish bash zsh" -d "Shell type"
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command completion' -s h -l help -d "Show help for completion"

# Status command - help option
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command status' -s h -l help -d "Show help for status"

# Checkout-PR command - help option
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command checkout-pr' -s h -l help -d "Show help for checkout-pr"

# Push command - help option
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command push' -s h -l help -d "Show help for push"

# Switch command - help option
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command switch' -s h -l help -d "Show help for switch"
