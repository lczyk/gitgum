#compdef __GITGUM_CMD__

_ff() {
    _arguments -s \
        '(-m --multi)'{-m,--multi}'[Allow selecting multiple items]' \
        '(-q --query)'{-q,--query}'=[Initial query]:query:_default' \
        '(-p --prompt)'{-p,--prompt}'=[Prompt prefix]:prompt:_default' \
        '--header=[Static header line]:header:_default' \
        '(-1 --select-1)'{-1,--select-1}'[Auto-select if exactly one match]' \
        '--fast[Disable streaming delay]' \
        '--reverse[Render prompt at top]' \
        '--height=[Number of rows to occupy]:rows:_default' \
        '--completion=[Print shell completion script]:shell:(bash fish zsh nu)' \
        '(-v --version)'{-v,--version}'[Show version]' \
        '(-h --help)'{-h,--help}'[Show help]'
}

if [ "$funcstack[1]" = "_ff" ]; then
    _ff "$@"
else
    compdef _ff __GITGUM_CMD__
fi
