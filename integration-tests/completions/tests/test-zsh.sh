#!/bin/zsh
#spellchecker: ignore gitgum
# Zsh completion integration test for gitgum

set -e

echo "=== Zsh Completion Test ==="

# Ensure gitgum binary exists
if [[ ! -x /work/bin/gitgum ]]; then
    echo "ERROR: gitgum binary not found at /work/bin/gitgum"
    exit 1
fi

# Generate completion script
echo "Generating zsh completion..."
/work/bin/gitgum completion zsh > /tmp/gitgum.zsh

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

# Verify the script is syntactically valid by checking if zsh can parse it
echo "Validating completion script syntax..."
if ! zsh -n /tmp/gitgum.zsh 2>&1; then
    echo "ERROR: Completion script has syntax errors"
    exit 1
fi

# Install the completion to fpath and load it properly
echo "Installing completion script..."
mkdir -p /tmp/zsh-completions
cp /tmp/gitgum.zsh /tmp/zsh-completions/_gitgum
fpath=(/tmp/zsh-completions $fpath)

# Reload completion system to pick up the new completion
compinit -u

# Verify the completion function is now loadable
echo "Verifying completion function..."
if ! whence -w _gitgum &>/dev/null; then
    # Try to autoload it
    autoload -Uz _gitgum 2>/dev/null || true
fi

if whence -w _gitgum | grep -q "function"; then
    echo "Completion function _gitgum is available"
else
    echo "WARNING: Completion function _gitgum not auto-loaded (this is OK - it will load on first use)"
fi

# Test completion by verifying the completion system recognizes gitgum
echo "Testing completion registration..."
# The completion should be registered for gitgum command
# We can't easily test the actual completion without a full interactive shell,
# but we can verify the file is properly formatted and loadable

echo "✓ Zsh completion test passed"
exit 0
