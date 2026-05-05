// Package matching filters a haystack of strings against a query using
// case-insensitive, whitespace-split substring matching: an item matches when
// it contains every whitespace-delimited needle from the query (any order).
package matching

import (
	"strings"
	"unicode/utf8"
)

// FindAll returns the indices of haystack entries that match query,
// preserving the original order. An empty query matches every item.
//
// Lowercases the query once per call; matches against haystack items
// case-insensitively without per-item allocations (no strings.ToLower).
// For hot-path callers that re-query the same haystack repeatedly,
// FindAllLower with a cached lowercased haystack is still faster
// (Boyer-Moore via strings.Contains vs. naive fold scan).
func FindAll(query string, haystack []string) []int {
	needles := strings.Fields(strings.ToLower(query))
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
func FindAllLower(lowerQuery string, lowerHaystack []string) []int {
	needles := strings.Fields(lowerQuery)
	res := make([]int, 0, len(lowerHaystack))
	for i, s := range lowerHaystack {
		if matches(s, needles) {
			res = append(res, i)
		}
	}
	return res
}

// matches reports whether every needle is a substring of itemLower.
func matches(itemLower string, lowerNeedles []string) bool {
	for _, n := range lowerNeedles {
		if !strings.Contains(itemLower, n) {
			return false
		}
	}
	return true
}

// matchesFold reports whether every pre-lowercased needle is a substring of
// s (mixed case), case-insensitively. Zero allocations.
func matchesFold(s string, lowerNeedles []string) bool {
	for _, n := range lowerNeedles {
		if !containsFold(s, n) {
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
