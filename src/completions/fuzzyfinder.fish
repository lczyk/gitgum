complete -c __GITGUM_CMD__ -s m -l multi -d 'Allow selecting multiple items'
complete -c __GITGUM_CMD__ -s q -l query -d 'Initial query' -r
complete -c __GITGUM_CMD__ -s p -l prompt -d 'Prompt prefix' -r
complete -c __GITGUM_CMD__ -l header -d 'Static header line' -r
complete -c __GITGUM_CMD__ -s 1 -l select-1 -d 'Auto-select if exactly one match'
complete -c __GITGUM_CMD__ -l fast -d 'Disable streaming delay'
complete -c __GITGUM_CMD__ -l reverse -d 'Render prompt at top'
complete -c __GITGUM_CMD__ -l height -d 'Number of rows to occupy' -r
complete -c __GITGUM_CMD__ -l completion -d 'Print shell completion script' -r -f -a 'bash fish zsh nu'
complete -c __GITGUM_CMD__ -s v -l version -d 'Show version'
complete -c __GITGUM_CMD__ -s h -l help -d 'Show help'
