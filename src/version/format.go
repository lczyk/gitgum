package version

//go:generate go run ../../cmd/generate-version

func FormatVersion(version, commitSHA, buildDate, buildInfo string) string {
	result := version

	if commitSHA != "" {
		result += "+" + commitSHA[:min(7, len(commitSHA))]
	}

	switch {
	case buildDate != "" && buildInfo != "":
		result += " (" + buildDate + ", " + buildInfo + ")"
	case buildDate != "":
		result += " (" + buildDate + ")"
	case buildInfo != "":
		result += " (" + buildInfo + ")"
	}

	return result
}
