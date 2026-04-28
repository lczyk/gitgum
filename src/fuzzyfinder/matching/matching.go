// Package matching filters a slice of strings against a query using
// case-insensitive, whitespace-split substring matching: an item matches when
// it contains every whitespace-delimited word in the query (any order).
package matching

import "strings"

// FindAll returns the indices of slice entries that match query, preserving
// the original order. An empty query matches every item.
//
// Lowercases the query and every item on each call. For hot-path callers
// that re-query the same corpus repeatedly (e.g. an interactive picker), use
// FindAllLower with a cached lowercased corpus to avoid per-keystroke
// allocations.
func FindAll(query string, slice []string) []int {
	words := strings.Fields(strings.ToLower(query))
	res := make([]int, 0, len(slice))
	for i, s := range slice {
		if matches(strings.ToLower(s), words) {
			res = append(res, i)
		}
	}
	return res
}

// FindAllLower is like FindAll but assumes query and items are already
// lowercase. Saves the per-call strings.ToLower allocations on the keystroke
// hot path.
func FindAllLower(lowerQuery string, lowerSlice []string) []int {
	words := strings.Fields(lowerQuery)
	res := make([]int, 0, len(lowerSlice))
	for i, s := range lowerSlice {
		if matches(s, words) {
			res = append(res, i)
		}
	}
	return res
}

func matches(itemLower string, lowerWords []string) bool {
	for _, w := range lowerWords {
		if !strings.Contains(itemLower, w) {
			return false
		}
	}
	return true
}
