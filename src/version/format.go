package version

import "strings"

//go:generate go run ./cmd/generate-version

func FormatVersion(version, commitSHA, buildDate, buildInfo string) string {
	result := version
	if commitSHA != "" {
		result += "+" + commitSHA[:min(7, len(commitSHA))]
	}
	var infoParts []string
	if buildDate != "" {
		infoParts = append(infoParts, buildDate)
	}
	if buildInfo != "" {
		infoParts = append(infoParts, buildInfo)
	}
	if len(infoParts) > 0 {
		return result + " (" + strings.Join(infoParts, ", ") + ")"
	}
	return result
}
