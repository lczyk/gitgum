package strutil

import "strings"

// SplitLines splits s on newlines, trims whitespace, and drops empty lines.
func SplitLines(s string) []string {
	var lines []string
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
