// Package matching filters a slice of strings against a query using
// case-insensitive, whitespace-split substring matching: an item matches when
// it contains every whitespace-delimited word in the query (any order).
package matching

import "strings"

// Matched represents a result of FindAll: the index of the matched item in
// the original slice.
type Matched struct {
	Idx int
}

// FindAll returns the indices of slice entries that match query, preserving
// the original order. An empty query matches every item.
func FindAll(query string, slice []string) []Matched {
	words := strings.Fields(strings.ToLower(query))
	res := make([]Matched, 0, len(slice))
	for i, s := range slice {
		if matches(strings.ToLower(s), words) {
			res = append(res, Matched{Idx: i})
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
