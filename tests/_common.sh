[[ -z "${__COMMON_SH__:-}" ]] && __COMMON_SH__=1 || return 0

__PROJECT_ROOT__="$(dirname "$PWD")"
# add src to PATH
export PATH="$__PROJECT_ROOT__/src:$PATH"
