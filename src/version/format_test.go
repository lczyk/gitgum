package version

import "testing"

func TestFormatVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		commitSHA string
		buildDate string
		buildInfo string
		want      string
	}{
		{
			name:    "version only",
			version: "1.0.0",
			want:    "1.0.0",
		},
		{
			name:      "version with commit",
			version:   "1.0.0",
			commitSHA: "abc1234567890",
			want:      "1.0.0+abc1234",
		},
		{
			name:      "version with commit and date",
			version:   "1.0.0",
			commitSHA: "abc1234567890",
			buildDate: "2026-01-01T00:00:00Z",
			want:      "1.0.0+abc1234 (2026-01-01T00:00:00Z)",
		},
		{
			name:      "version with commit, date, and dirty",
			version:   "1.0.0",
			commitSHA: "abc1234567890",
			buildDate: "2026-01-01T00:00:00Z",
			buildInfo: "dirty",
			want:      "1.0.0+abc1234 (2026-01-01T00:00:00Z, dirty)",
		},
		{
			name:      "dirty without date",
			version:   "1.0.0",
			commitSHA: "abc1234567890",
			buildInfo: "dirty",
			want:      "1.0.0+abc1234 (dirty)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatVersion(tt.version, tt.commitSHA, tt.buildDate, tt.buildInfo)
			if got != tt.want {
				t.Errorf("FormatVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
