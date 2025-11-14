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
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a tree -d "Show the git tree structure"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a push -d "Push the current branch to a remote repository"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a delete -d "Delete a branch or a remote branch"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a status -d "Show the status of the current git repository"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a commit -d "Commit changes in the current branch"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a switch -d "Switch to a branch interactively"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a merge-into -d "Merge current branch into another branch"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a completion -d "Output shell completion script"
complete -f -c __GITGUM_CMD__ -n __fish_gitgum_needs_command -a help -d "Show help message"

# Global options
complete -f -c __GITGUM_CMD__ -s h -l help -d "Show help message"

# Command-specific options
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command push' -s n -l dry-run -d "Perform a dry run"
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command push' -s h -l help -d "Show help for push"

complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command tree' -s h -l help -d "Show help for tree"

complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command delete' -s n -l dry-run -d "Perform a dry run"
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command delete' -s h -l help -d "Show help for delete"

complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command status' -s h -l help -d "Show help for status"

complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command commit' -s n -l dry-run -d "Perform a dry run"
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command commit' -s h -l help -d "Show help for commit"

complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command switch' -s n -l dry-run -d "Perform a dry run"
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command switch' -s h -l help -d "Show help for switch"

complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command merge-into' -s n -l dry-run -d "Perform a dry run"
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command merge-into' -s h -l help -d "Show help for merge-into"

# Completion command - suggest shell types
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command completion' -a "fish bash zsh" -d "Shell type"
complete -f -c __GITGUM_CMD__ -n '__fish_gitgum_using_command completion' -s h -l help -d "Show help for completion"
