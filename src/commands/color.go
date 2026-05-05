package commands

import (
	"os"
	"strconv"
)

// stdoutIsTTY reports whether os.Stdout is a character device (terminal).
func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// colorEnabled reports whether ansi escapes should be emitted.
//
// precedence:
//   - FORCE_COLOR set + parses as bool -> that value wins
//   - FORCE_COLOR set + unparsable     -> true (chalk-style force on)
//   - NO_COLOR set (any non-empty)     -> false (per no-color.org)
//   - else: stdout is a char device    -> true
func colorEnabled() bool {
	if fc := os.Getenv("FORCE_COLOR"); fc != "" {
		if v, err := strconv.ParseBool(fc); err == nil {
			return v
		}
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return stdoutIsTTY()
}

// paint wraps s in the given ansi code + reset, iff color is enabled.
func paint(code, s string) string {
	if !colorEnabled() {
		return s
	}
	return code + s + ansiReset
}
