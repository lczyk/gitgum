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

	if buildDate != "" || buildInfo != "" {
		result.WriteString(" (")
		if buildDate != "" {
			result.WriteString(buildDate)
			if buildInfo != "" {
				result.WriteString(", ")
			}
		}
		if buildInfo != "" {
			result.WriteString(buildInfo)
		}
		result.WriteByte(')')
	}

	return result.String()
}
