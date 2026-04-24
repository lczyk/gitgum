package version

func GetFullVersion() string {
	version := Version
	if CommitSHA != "" {
		version += "+" + CommitSHA[:7]
	}
	info := ""
	if BuildDate != "" {
		info = BuildDate
	}
	if BuildInfo != "" {
		if info != "" {
			info += ", " + BuildInfo
		} else {
			info = BuildInfo
		}
	}
	if info != "" {
		return version + " (" + info + ")"
	}
	return version
}
