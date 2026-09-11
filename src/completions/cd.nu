# worktree-switch prints a path; only the shell itself can cd, so the binary is
# wrapped: the long form as its own command (nu resolves the two-word name
# before the one-word one, so this shadows the completion extern), the `w`
# alias through the top-level one. Help / version output must stay output, not
# a cd target.
#
# The marker `gg doctor` looks for is set here, at source time, rather than
# per call as the other shells do: nu resolves `gg status` to the completion
# extern of that name before it considers this one-word command, so most calls
# never pass through it.
$env.GITGUM_CD_WRAPPER = "1"

module gitgum-cd {

  # numbers of the worktrees named <main>/<main>-N beside the main one, 1 first
  def "nu-complete gitgum worktree numbers" [] {
    let names = (^git worktree list --porcelain | lines | where ($it | str starts-with "worktree ") | each {|l| $l | str substring 9.. | path basename })
    if ($names | is-empty) { return [] }
    let main = ($names | first)
    $names | each {|b|
      if $b == $main {
        "1"
      } else if ($b | str starts-with $"($main)-") {
        let n = ($b | str substring (($main | str length) + 1)..)
        if ($n =~ '^[0-9]+$') { $n } else { null }
      } else {
        null
      }
    } | compact
  }

  export def --env --wrapped "__GITGUM_CMD__" [...args: string] {
    if ($args | length) > 0 and ($args | first) in ["w" "worktree-switch"] {
      __GITGUM_CMD__ worktree-switch ...($args | skip 1)
    } else {
      ^__GITGUM_CMD__ ...$args
    }
  }

  # cd to worktree N, or cycle to the next one
  export def --env --wrapped "__GITGUM_CMD__ worktree-switch" [
    ...args: string@"nu-complete gitgum worktree numbers" # Worktree number (1 is <repo>, N is <repo>-N); omit to cycle
  ] {
    if ($args | any {|a| $a in ["-h" "--help" "-v" "--version"]}) {
      ^__GITGUM_CMD__ worktree-switch ...$args
    } else {
      cd (^__GITGUM_CMD__ worktree-switch ...$args)
    }
  }

}

export use gitgum-cd *
