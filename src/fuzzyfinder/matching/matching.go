// Package matching filters a haystack of strings against a query using
// case-insensitive, whitespace-split substring matching: an item matches when
// it contains every whitespace-delimited needle from the query (any order).
package matching

import "strings"

// FindAll returns the indices of haystack entries that match query,
// preserving the original order. An empty query matches every item.
//
// Lowercases the query and every haystack item on each call. For hot-path
// callers that re-query the same haystack repeatedly (e.g. an interactive
// picker), use FindAllLower with a cached lowercased haystack to avoid
// per-keystroke allocations.
func FindAll(query string, haystack []string) []int {
	needles := strings.Fields(strings.ToLower(query))
	res := make([]int, 0, len(haystack))
	for i, s := range haystack {
		if matches(strings.ToLower(s), needles) {
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
