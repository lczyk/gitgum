#!/bin/bash
#spellchecker: ignore gitgum

# Bash completion integration test for gitgum

set -euo pipefail

echo "=== Bash Completion Test ==="

# Ensure gitgum binary exists
if [[ ! -x /work/bin/gitgum ]]; then
    echo "ERROR: gitgum binary not found at /work/bin/gitgum"
    exit 1
fi

# Generate completion script
echo "Generating bash completion..."
/work/bin/gitgum completion bash > /tmp/gitgum.bash

# Verify completion file exists and has content
if [[ ! -s /tmp/gitgum.bash ]]; then
    echo "ERROR: Generated completion file is empty"
    exit 1
fi

# Check for key completion markers
echo "Checking for completion markers..."
if ! grep -q "_gitgum_completion" /tmp/gitgum.bash; then
    echo "ERROR: Missing _gitgum_completion function"
    exit 1
fi

if ! grep -q "complete.*gitgum" /tmp/gitgum.bash; then
    echo "ERROR: Missing complete command registration"
    exit 1
fi

# Verify placeholder replacement (should NOT contain __GITGUM_CMD__)
if grep -q "__GITGUM_CMD__" /tmp/gitgum.bash; then
    echo "ERROR: Placeholder __GITGUM_CMD__ not replaced"
    exit 1
fi

# Source the bash-completion framework first (provides _init_completion)
echo "Loading bash-completion framework..."
if [[ -f /usr/share/bash-completion/bash_completion ]]; then
    source /usr/share/bash-completion/bash_completion
elif [[ -f /etc/bash_completion ]]; then
    source /etc/bash_completion
else
    echo "WARNING: bash-completion framework not found, some features may not work"
fi

# Source the completion script
echo "Sourcing completion script..."
# Load bash-completion framework first
source /usr/share/bash-completion/bash_completion 2>/dev/null || true
source /tmp/gitgum.bash

# Verify the completion function exists
if ! declare -F _gitgum_completion >/dev/null; then
    echo "ERROR: Completion function _gitgum_completion not defined after sourcing"
    exit 1
fi

# Test completion hook invocation
echo "Testing completion hook..."
# Set up bash completion variables as they would be during tab completion
COMP_WORDS=(gitgum switch)
COMP_CWORD=1
COMP_LINE="gitgum switch"
COMP_POINT=${#COMP_LINE}

# Call the completion function
if ! _gitgum_completion; then
    echo "ERROR: Completion function failed to execute"
    exit 1
fi

# Check that COMPREPLY was populated (it should contain at least something)
if [[ ${#COMPREPLY[@]} -eq 0 ]]; then
    echo "WARNING: COMPREPLY is empty (may be expected if no matches)"
fi
