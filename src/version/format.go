package version

func GetFullVersion() string {
	return FormatVersion(Version, CommitSHA, BuildDate, BuildInfo)
}

func FormatVersion(version, commitSHA, buildDate, buildInfo string) string {
	if commitSHA != "" {
		version += "+" + commitSHA[:7]
	}
	info := ""
	if buildDate != "" {
		info = buildDate
	}
	if buildInfo != "" {
		if info != "" {
			info += ", " + buildInfo
		} else {
			info = buildInfo
		}
	}
	if info != "" {
		return version + " (" + info + ")"
	}
	return version
}
