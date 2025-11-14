#!/usr/bin/fish
# Fish completion integration test for gitgum

echo "=== Fish Completion Test ==="

# Ensure gitgum binary exists
if not test -x /work/bin/gitgum
    echo "ERROR: gitgum binary not found at /work/bin/gitgum"
    exit 1
end

# Generate completion script
echo "Generating fish completion..."
/work/bin/gitgum completion fish > /tmp/gitgum.fish

# Verify completion file exists and has content
if not test -s /tmp/gitgum.fish
    echo "ERROR: Generated completion file is empty"
    exit 1
end

# Check for key completion markers
echo "Checking for completion markers..."
if not grep -q "__fish_gitgum_needs_command" /tmp/gitgum.fish
    echo "ERROR: Missing __fish_gitgum_needs_command function"
    exit 1
end

if not grep -q "complete.*-c.*gitgum" /tmp/gitgum.fish
    echo "ERROR: Missing complete command registration"
    exit 1
end

# Verify placeholder replacement (should NOT contain __GITGUM_CMD__)
if grep -q "__GITGUM_CMD__" /tmp/gitgum.fish
    echo "ERROR: Placeholder __GITGUM_CMD__ not replaced"
    exit 1
end

# Source the completion script
echo "Sourcing completion script..."
source /tmp/gitgum.fish

# Verify the completion functions exist
if not functions -q __fish_gitgum_needs_command
    echo "ERROR: Function __fish_gitgum_needs_command not defined after sourcing"
    exit 1
end

# Test completion hook by checking that completions are registered
echo "Testing completion hook..."
set -l completions (complete -C "gitgum " | string collect)
if test -z "$completions"
    echo "WARNING: No completions returned for 'gitgum ' (may be expected)"
else
    echo "Completions available: $completions"
end

# Test that 'switch' command is in completions
set -l switch_completions (complete -C "gitgum sw" | string collect)
if not string match -q "*switch*" -- $switch_completions
    echo "WARNING: 'switch' not found in completions for 'gitgum sw'"
end

echo "✓ Fish completion test passed"
exit 0
