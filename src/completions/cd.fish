# worktree-switch prints a path; only the shell itself can cd, so the binary is
# wrapped. Help / version output must stay output, not a cd target. Every other
# call passes through with a marker so `gg doctor` can tell the wrapper is on.
function __GITGUM_CMD__ --wraps __GITGUM_CMD__ --description 'gitgum, with worktree-switch changing directory'
    switch "$argv[1]"
        case w w+ w- worktree-switch worktree-switch+ worktree-switch-
            if contains -- -h $argv; or contains -- --help $argv; or contains -- -v $argv; or contains -- --version $argv
                command __GITGUM_CMD__ $argv
                return
            end
            set -l target (command __GITGUM_CMD__ $argv)
            and cd $target
        case '*'
            GITGUM_CD_WRAPPER=1 command __GITGUM_CMD__ $argv
    end
end
