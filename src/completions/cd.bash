# worktree-switch prints a path; only the shell itself can cd, so the binary is
# wrapped. Help / version output must stay output, not a cd target. Every other
# call passes through with a marker so `gg doctor` can tell the wrapper is on.
__GITGUM_CMD__() {
    case "$1" in
        w|w+|w-|worktree-switch|worktree-switch+|worktree-switch-)
            local arg target
            for arg in "$@"; do
                case "$arg" in
                    -h|--help|-v|--version)
                        command __GITGUM_CMD__ "$@"
                        return
                        ;;
                esac
            done
            target="$(command __GITGUM_CMD__ "$@")" && cd "$target"
            ;;
        *)
            GITGUM_CD_WRAPPER=1 command __GITGUM_CMD__ "$@"
            ;;
    esac
}
