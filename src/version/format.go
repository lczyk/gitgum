package version

import "strings"

//go:generate go run ./cmd/generate-version

func FormatVersion(version, commitSHA, buildDate, buildInfo string) string {
	var result strings.Builder
	result.WriteString(version)

	if commitSHA != "" {
		result.WriteByte('+')
		result.WriteString(commitSHA[:min(7, len(commitSHA))])
	}

	var infoParts []string
	if buildDate != "" {
		infoParts = append(infoParts, buildDate)
	}
	if buildInfo != "" {
		infoParts = append(infoParts, buildInfo)
	}

	if len(infoParts) > 0 {
		result.WriteString(" (")
		result.WriteString(strings.Join(infoParts, ", "))
		result.WriteByte(')')
	}

	return result.String()
}
