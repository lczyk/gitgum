package strutil

import "strings"

// SplitTrimmedLines splits s on newlines, trims whitespace from each line, and
// drops the empty ones. The trimming is why the name says so: it would eat the
// leading space of a porcelain status code, so anything parsing those wants a
// raw split instead.
func SplitTrimmedLines(s string) []string {
	var lines []string
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
