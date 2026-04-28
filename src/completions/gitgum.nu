module completions {

  def "nu-complete gitgum shell" [] {
    [ "bash" "fish" "zsh" "nu" ]
  }

  def "nu-complete gitgum bump" [] {
    [ "patch" "minor" "major" ]
  }

  def "nu-complete gitgum branches" [] {
    ^git for-each-ref --format='%(refname:short)' refs/heads refs/remotes | lines
  }

  export extern "__GITGUM_CMD__" [
    --help(-h)               # Show help
    --version(-v)            # Show version
  ]

  export extern "__GITGUM_CMD__ switch" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ checkout-pr" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ completion" [
    shell: string@"nu-complete gitgum shell" # Shell type
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ status" [
    --help(-h)               # Show help
  ]

  export extern "__GITGUM_CMD__ push" [
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
