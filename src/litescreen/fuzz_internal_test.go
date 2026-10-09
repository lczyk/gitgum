package litescreen

import (
	"bytes"
	"testing"
)

// isSubsequence reports whether sub can be formed from in by deleting bytes.
func isSubsequence(sub, in []byte) bool {
	j := 0
	for i := 0; i < len(in) && j < len(sub); i++ {
		if in[i] == sub[j] {
			j++
		}
	}
	return j == len(sub)
}

// FuzzSplitCPR pins the shape of the type-ahead split: what comes back is the
// input minus whole cursor-position reports, and nothing is invented. It does
// not ask for idempotence: cutting a report out of "\x1b[5;1\x1b[5;1RR" leaves
// a fresh one spliced from the fragments, which no terminal produces and which
// the key parser drops as an unknown CSI anyway.
func FuzzSplitCPR(f *testing.F) {
	for _, seed := range []string{
		"", "R", "\x1b", "\x1b[", "\x1b[12;", "\x1b[12;1R", "ab\x1b[24;1Rcd",
		"\x1b[A\x1b[3;4R", "\x1b[3;1R\x1b[9;1R", "\x1b[;1R", "\x1b[1;R", "\x1b[0;0R",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		row, rest := splitCPR(in)
		if row < 0 {
			t.Fatalf("negative row %d from %q", row, in)
		}
		if len(rest) > len(in) || !isSubsequence(rest, in) {
			t.Fatalf("rest %q is not a subsequence of %q", rest, in)
		}
		if row > 0 && len(rest) == len(in) {
			t.Fatalf("row %d reported but nothing removed from %q", row, in)
		}
		if row == 0 && !bytes.Equal(rest, in) && !bytes.Contains(in, []byte("R")) {
			t.Fatalf("bytes removed from %q without a report in it: %q", in, rest)
		}
	})
}
