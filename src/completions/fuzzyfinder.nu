module completions {

  def "nu-complete ff shell" [] {
    [ "bash" "fish" "zsh" "nu" ]
  }

  export extern "__GITGUM_CMD__" [
    --multi(-m)              # Allow selecting multiple items
    --query(-q): string      # Initial query
    --prompt(-p): string     # Prompt prefix
    --header: string         # Static header line
    --select-1               # Auto-select if exactly one match
    --fast                   # Disable streaming delay
    --reverse                # Render prompt at top
    --height: int            # Number of rows to occupy
    --completion: string@"nu-complete ff shell" # Print shell completion script
    --version(-v)            # Show version
    --help(-h)               # Show help
  ]

}

export use completions *
