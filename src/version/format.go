package version

import "strings"

func GetFullVersion() string {
	return FormatVersion(Version, CommitSHA, BuildDate, BuildInfo)
}

func FormatVersion(version, commitSHA, buildDate, buildInfo string) string {
	if commitSHA != "" {
		version += "+" + commitSHA[:min(7, len(commitSHA))]
	}
	var infoParts []string
	if buildDate != "" {
		infoParts = append(infoParts, buildDate)
	}
	if buildInfo != "" {
		infoParts = append(infoParts, buildInfo)
	}
	if len(infoParts) > 0 {
		return version + " (" + strings.Join(infoParts, ", ") + ")"
	}
	return version
}
