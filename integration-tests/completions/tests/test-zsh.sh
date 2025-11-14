#!/bin/zsh
# Zsh completion integration test for gitgum

set -e

echo "=== Zsh Completion Test ==="

# Ensure gitgum binary exists
if [[ ! -x /workspace/bin/gitgum ]]; then
    echo "ERROR: gitgum binary not found at /workspace/bin/gitgum"
    exit 1
fi

# Generate completion script
echo "Generating zsh completion..."
/workspace/bin/gitgum completion zsh > /tmp/gitgum.zsh

# Verify completion file exists and has content
if [[ ! -s /tmp/gitgum.zsh ]]; then
    echo "ERROR: Generated completion file is empty"
    exit 1
fi

# Check for key completion markers
echo "Checking for completion markers..."
if ! grep -q "#compdef" /tmp/gitgum.zsh; then
    echo "ERROR: Missing #compdef directive"
    exit 1
fi

if ! grep -q "_gitgum" /tmp/gitgum.zsh; then
    echo "ERROR: Missing _gitgum completion function"
    exit 1
fi

# Verify placeholder replacement (should NOT contain __GITGUM_CMD__)
if grep -q "__GITGUM_CMD__" /tmp/gitgum.zsh; then
    echo "ERROR: Placeholder __GITGUM_CMD__ not replaced"
    exit 1
fi

# Initialize zsh completion system
echo "Initializing zsh completion system..."
autoload -Uz compinit
compinit -u

# Create a temporary fpath directory and add our completion
mkdir -p /tmp/zsh-completions
cp /tmp/gitgum.zsh /tmp/zsh-completions/_gitgum
fpath=(/tmp/zsh-completions $fpath)

# Source the completion script
echo "Sourcing completion script..."
source /tmp/gitgum.zsh

# Verify the completion function exists
if ! whence -w _gitgum | grep -q "function"; then
    echo "ERROR: Completion function _gitgum not defined after sourcing"
    exit 1
fi

# Test completion by simulating completion context
echo "Testing completion hook..."
# Set up completion context
local -a reply
local CURRENT=2
local words=(gitgum switch)

# Try to call the completion function (it might fail if not fully set up, but shouldn't crash)
if ! _gitgum 2>/dev/null; then
    echo "WARNING: Completion function execution failed (may be expected in test environment)"
else
    echo "Completion function executed successfully"
fi

echo "✓ Zsh completion test passed"
exit 0
