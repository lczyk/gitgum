function __fish_gitgum_no_subcommand
    not __fish_seen_subcommand_from clone switch branch checkout-pr add-remote completion status push pull tree diff doctor clean delete replay-list empty release
end

function __fish_gitgum_branches
    git for-each-ref --format='%(refname:short)' refs/heads refs/remotes 2>/dev/null
end

# Subcommands
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a clone -d "Clone a repository, applying gg doctor's naming rules"
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a switch -d 'Switch to a branch interactively'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a branch -d 'Create a new branch off an existing one and switch to it'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a checkout-pr -d 'Checkout a pull request from a remote repository'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a add-remote -d "Add a remote, applying gg doctor's naming rules"
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a completion -d 'Output shell completion script'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a status -d 'Show the status of the current git repository'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a push -d 'Push the current branch to a remote repository'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a pull -d "Fetch and integrate the current branch's upstream"
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a tree -d 'Print a colored commit graph across all branches'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a diff -d 'Show working-tree diff with --compact-summary'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a doctor -d "Diagnose known inconsistencies in the repo's remote/worktree layout"
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a clean -d 'Discard working tree changes and untracked files'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a delete -d 'Delete a local branch and optionally its remote tracking branch'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a replay-list -d 'List commits on branch A since divergence from trunk B'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a empty -d 'Create an empty commit and optionally push it'
complete -c __GITGUM_CMD__ -n __fish_gitgum_no_subcommand -f -a release -d 'Bump VERSION (or latest tag), commit, and tag'

# Global flags
complete -c __GITGUM_CMD__ -s h -l help -d 'Show help'
complete -c __GITGUM_CMD__ -s v -l version -d 'Show version'

# clone flags
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from clone' -l depth -d 'Create a shallow clone with the given history depth'

# tree flags
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from tree' -l since -d 'Limit to commits since <expr> (empty for all history)'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from tree' -s a -l all -d 'Show the full history'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from tree' -s f -l follow -d 'Follow mode: refresh every N seconds'

# diff flags
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from diff' -s m -l mode -d 'Lock to a diff level: work, index, untracked, head'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from diff' -s f -l follow -d 'Follow mode: refresh every N seconds'

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

# status <sections>
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from status' -f -a all -d 'Every section'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from status' -f -a branch -d 'Local branches'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from status' -f -a remote -d 'Configured remotes'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from status' -f -a worktree -d 'Linked worktrees'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from status' -f -a changes -d 'Working-tree changes'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from status' -f -a head -d 'HEAD summary line'

# status flags
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from status' -s a -l all -d 'Render every section'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from status' -l flat -d 'Flat porcelain list instead of tree'
complete -c __GITGUM_CMD__ -n '__fish_seen_subcommand_from status' -s f -l follow -d 'Follow mode: refresh every N seconds (default 2)'
