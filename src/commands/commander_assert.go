package commands

import flags "github.com/jessevdk/go-flags"

// Compile-time assertions that every command implements flags.Commander.
// Without this, a wrong Execute signature silently becomes a no-op at runtime.
var (
	_ flags.Commander = (*StatusCommand)(nil)
	_ flags.Commander = (*SwitchCommand)(nil)
	_ flags.Commander = (*CheckoutPRCommand)(nil)
	_ flags.Commander = (*CompletionCommand)(nil)
	_ flags.Commander = (*PushCommand)(nil)
	_ flags.Commander = (*CleanCommand)(nil)
	_ flags.Commander = (*DeleteCommand)(nil)
	_ flags.Commander = (*ReplayListCommand)(nil)
	_ flags.Commander = (*EmptyCommand)(nil)
)
