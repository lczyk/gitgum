package commands

import (
	"os"
	"strconv"
)

// isTTY reports whether f is a character device (terminal).
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func stdoutIsTTY() bool { return isTTY(os.Stdout) }

// colorEnabled reports whether ansi escapes should be emitted.
//
// precedence:
//   - FORCE_COLOR set + parses as bool -> that value wins
//   - FORCE_COLOR set + unparsable     -> true (chalk-style force on)
//   - NO_COLOR set (any non-empty)     -> false (per no-color.org)
//   - else: stdout is a char device    -> true
func colorEnabled() bool { return colorEnabledOn(os.Stdout) }

// colorEnabledOn is colorEnabled for output that lands on f instead of stdout.
func colorEnabledOn(f *os.File) bool {
	if fc := os.Getenv("FORCE_COLOR"); fc != "" {
		if v, err := strconv.ParseBool(fc); err == nil {
			return v
		}
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTTY(f)
}

// paint wraps s in the given ansi code + reset, iff color is enabled.
func paint(code, s string) string {
	if !colorEnabled() {
		return s
	}
	return code + s + ansiReset
}
