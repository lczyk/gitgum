package strutil

import "strings"

// SplitLines splits s on newlines, trims whitespace, and drops empty lines.
func SplitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
