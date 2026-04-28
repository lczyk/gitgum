function __fish_gitgum_no_subcommand
    not __fish_seen_subcommand_from switch checkout-pr completion status push clean delete replay-list empty release
end

function __fish_gitgum_branches
    git for-each-ref --format='%(refname:short)' refs/heads refs/remotes 2>/dev/null
end

# Subcommands
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a switch -d 'Switch to a branch interactively'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a checkout-pr -d 'Checkout a pull request from a remote repository'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a completion -d 'Output shell completion script'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a status -d 'Show the status of the current git repository'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a push -d 'Push the current branch to a remote repository'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a clean -d 'Discard working tree changes and untracked files'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a delete -d 'Delete a local branch and optionally its remote tracking branch'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a replay-list -d 'List commits on branch A since divergence from trunk B'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a empty -d 'Create an empty commit and optionally push it'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a release -d 'Bump VERSION (or latest tag), commit, and tag'

# Global flags
complete -c __GITGUM_CMD__ -s h -l help -d 'Show help'
complete -c __GITGUM_CMD__ -s v -l version -d 'Show version'

# completion <shell>
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from completion' -f -a 'bash' -d 'Bourne Again SHell'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from completion' -f -a 'fish' -d 'Friendly Interactive SHell'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from completion' -f -a 'zsh' -d 'Z shell'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from completion' -f -a 'nu' -d 'Nushell'

# clean flags
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from clean' -l changes -d 'Discard staged and unstaged changes (default: true)'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from clean' -l untracked -d 'Remove untracked files (default: true)'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from clean' -l ignored -d 'Remove ignored files (default: false)'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from clean' -l all -d 'Enable all cleanup options'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from clean' -s y -l yes -d 'Skip confirmation prompt'

# replay-list <A> <B>
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from replay-list' -f -a '(__fish_gitgum_branches)' -d 'Branch'

# release <bump>
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from release' -f -a 'patch' -d 'Patch version bump'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from release' -f -a 'minor' -d 'Minor version bump'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from release' -f -a 'major' -d 'Major version bump'
