// Package matching filters a haystack of strings against a query using
// case-insensitive, whitespace-split substring matching: an item matches when
// it contains every whitespace-delimited needle from the query (any order).
//
// When negation is enabled, a needle prefixed with '!' inverts: the item must
// NOT contain the rest of that needle (fzf-style "!foo"). A bare "!" is ignored.
package matching

import (
	"strings"
	"unicode/utf8"
)

// needle is one parsed query term. neg flips the contains test.
type needle struct {
	text string // already lowercase; never empty
	neg  bool
}

// parse splits lowerQuery into needles. When negate, a leading '!' marks the
// needle negative; a bare "!" (empty text) is dropped.
func parse(lowerQuery string, negate bool) []needle {
	fields := strings.Fields(lowerQuery)
	needles := make([]needle, 0, len(fields))
	for _, f := range fields {
		if negate && f[0] == '!' {
			if len(f) == 1 {
				continue
			}
			needles = append(needles, needle{text: f[1:], neg: true})
			continue
		}
		needles = append(needles, needle{text: f})
	}
	return needles
}

// FindAll returns the indices of haystack entries that match query,
// preserving the original order. An empty query matches every item.
// negate enables fzf-style '!' negative needles.
//
// Lowercases the query once per call; matches against haystack items
// case-insensitively without per-item allocations (no strings.ToLower).
// For hot-path callers that re-query the same haystack repeatedly,
// FindAllLower with a cached lowercased haystack is still faster
// (Boyer-Moore via strings.Contains vs. naive fold scan).
func FindAll(query string, haystack []string, negate bool) []int {
	needles := parse(strings.ToLower(query), negate)
	res := make([]int, 0, len(haystack))
	for i, s := range haystack {
		if matchesFold(s, needles) {
			res = append(res, i)
		}
	}
	return res
}

// FindAllLower is like FindAll but assumes query and haystack are already
// lowercase. Saves the per-call strings.ToLower allocations on the keystroke
// hot path.
func FindAllLower(lowerQuery string, lowerHaystack []string, negate bool) []int {
	needles := parse(lowerQuery, negate)
	res := make([]int, 0, len(lowerHaystack))
	for i, s := range lowerHaystack {
		if matches(s, needles) {
			res = append(res, i)
		}
	}
	return res
}

// matches reports whether itemLower satisfies every needle.
func matches(itemLower string, needles []needle) bool {
	for _, n := range needles {
		if strings.Contains(itemLower, n.text) == n.neg {
			return false
		}
	}
	return true
}

// matchesFold reports whether s (mixed case) satisfies every needle,
// case-insensitively. Zero allocations.
func matchesFold(s string, needles []needle) bool {
	for _, n := range needles {
		if containsFold(s, n.text) == n.neg {
			return false
		}
	}
	return true
}

// containsFold reports whether lowerSubstr (already lowercase) is a substring
// of s (mixed case), case-insensitively. ASCII-only strings use a byte-level
// fast path (zero allocs). If either string contains non-ASCII bytes, falls
// back to strings.ToLower + strings.Contains — rare in typical branch /
// file-name workloads, so the alloc hit is negligible.
func containsFold(s, lowerSubstr string) bool {
	if len(lowerSubstr) == 0 {
		return true
	}
	if len(lowerSubstr) > len(s) {
		return false
	}
	n := len(lowerSubstr)
	end := len(s) - n
	for start := 0; start <= end; start++ {
		matched := true
		for j := 0; j < n; j++ {
			a := s[start+j]
			b := lowerSubstr[j]
			if a == b {
				continue
			}
			if a >= utf8.RuneSelf || b >= utf8.RuneSelf {
				goto fallback
			}
			if a >= 'A' && a <= 'Z' && a+('a'-'A') == b {
				continue
			}
			matched = false
			break
		}
		if matched {
			return true
		}
	}
	return false

fallback:
	return strings.Contains(strings.ToLower(s), lowerSubstr)
}
