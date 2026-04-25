package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lczyk/assert"
)

func TestParseSemver(t *testing.T) {
	tests := []struct {
		in      string
		want    semver
		wantErr bool
	}{
		{"1.2.3", semver{1, 2, 3}, false},
		{"0.0.0", semver{0, 0, 0}, false},
		{"10.20.30", semver{10, 20, 30}, false},
		{"1.2", semver{}, true},
		{"1.2.3.4", semver{}, true},
		{"a.b.c", semver{}, true},
		{"1.2.x", semver{}, true},
	}
	for _, tt := range tests {
		got, err := parseSemver(tt.in)
		if tt.wantErr {
			assert.Error(t, err, assert.AnyError, tt.in)
		} else {
			assert.NoError(t, err, tt.in)
			assert.Equal(t, got, tt.want)
		}
	}
}

func TestBumpVersion(t *testing.T) {
	tests := []struct {
		in, bump, want string
	}{
		{"1.2.3", "patch", "1.2.4"},
		{"1.2.3", "minor", "1.3.0"},
		{"1.2.3", "major", "2.0.0"},
		{"0.0.9", "patch", "0.0.10"},
		{"1.9.9", "minor", "1.10.0"},
	}
	for _, tt := range tests {
		got, err := bumpVersion(tt.in, tt.bump)
		assert.NoError(t, err, tt.in+"/"+tt.bump)
		assert.Equal(t, got, tt.want)
	}
}

func TestBumpVersion_UnknownBump(t *testing.T) {
	_, err := bumpVersion("1.2.3", "invalid")
	assert.Error(t, err, assert.AnyError, "unknown bump should error")
}

func TestReadWriteVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "VERSION")

	// round-trip with header
	header := []string{"# managed by release command"}
	err := writeVersion(path, header, "1.2.3")
	assert.NoError(t, err, "writeVersion")

	gotHeader, gotVer, err := readVersion(path)
	assert.NoError(t, err, "readVersion")
	assert.Equal(t, gotVer, "1.2.3")
	assert.EqualArrays(t, gotHeader, header)

	// round-trip without header
	err = writeVersion(path, nil, "2.0.0")
	assert.NoError(t, err, "writeVersion no header")

	_, gotVer, err = readVersion(path)
	assert.NoError(t, err, "readVersion no header")
	assert.Equal(t, gotVer, "2.0.0")

	// missing file
	_, _, err = readVersion(filepath.Join(dir, "MISSING"))
	assert.Error(t, err, assert.AnyError, "missing file should error")

	// file with no version line
	empty := filepath.Join(dir, "EMPTY")
	os.WriteFile(empty, []byte("# only a comment\n"), 0o644)
	_, _, err = readVersion(empty)
	assert.Error(t, err, assert.AnyError, "no version line should error")
}
