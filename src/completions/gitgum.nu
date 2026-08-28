module completions {

  def "nu-complete gitgum shell" [] {
    [ "bash" "fish" "zsh" "nu" ]
  }

  def "nu-complete gitgum bump" [] {
    [ "patch" "minor" "major" ]
  }

  def "nu-complete gitgum sections" [] {
    [ "all" "branch" "remote" "worktree" "changes" "head" ]
  }

  def "nu-complete gitgum branches" [] {
    ^git for-each-ref --format='%(refname:short)' refs/heads refs/remotes | lines
  }

  export extern "__GITGUM_CMD__" [
    --help(-h)               # Show help
    --version(-v)            # Show version
  ]

  export extern "__GITGUM_CMD__ clone" [
    url: string              # Repo url or user/repo shorthand
    dir?: string             # Target directory (default: repo name)
    --depth: int             # Create a shallow clone with the given history depth
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ switch" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ branch" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ checkout-pr" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ add-remote" [
    remote: string           # Repo url, forge/user/repo, or user/repo shorthand
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ completion" [
    shell: string@"nu-complete gitgum shell" # Shell type
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ status" [
    sections?: string@"nu-complete gitgum sections" # Comma-separated sections (default: changes,head)
    --all(-a)                # Render every section
    --flat                   # Flat porcelain list instead of tree
    --follow(-f): float      # Follow mode: refresh every N seconds (default 2)
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ push" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ pull" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ tree" [
    --since: string          # Limit to commits since <expr> (empty for all history)
    --all(-a)                # Show the full history
    --follow(-f): float      # Follow mode: refresh every N seconds
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ diff" [
    --mode(-m): string       # Lock to a diff level: work, index, untracked, head
    --follow(-f): float      # Follow mode: refresh every N seconds
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ doctor" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ clean" [
    --changes                # Discard staged and unstaged changes (default: true)
    --untracked              # Remove untracked files (default: true)
    --ignored                # Remove ignored files (default: false)
    --all                    # Enable all cleanup options
    --yes(-y)                # Skip confirmation prompt
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ delete" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ replay-list" [
    branch_a: string@"nu-complete gitgum branches" # Feature branch with commits to list
    branch_b: string@"nu-complete gitgum branches" # Trunk/base branch
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ empty" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ release" [
    bump: string@"nu-complete gitgum bump" # Version bump level
    --help(-h)               # Show help
  ]

}

export use completions *
