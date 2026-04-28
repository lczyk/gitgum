package fuzzyfinder

// itemKey identifies an item by its text and 0-based occurrence count among
// equal predecessors. This lets the picker preserve cursor and selection
// state across resyncs even when the underlying slice shrinks, grows, or
// contains duplicates.
type itemKey struct {
	text string
	nth  int
}

// keyOf returns the identity key for items[idx].
func keyOf(items []string, idx int) itemKey {
	if idx < 0 || idx >= len(items) {
		return itemKey{}
	}
	text := items[idx]
	nth := 0
	for i := range idx {
		if items[i] == text {
			nth++
		}
	}
	return itemKey{text: text, nth: nth}
}

// findKey returns the items index for the nth occurrence of key.text, or
// (-1, false) when absent.
func findKey(items []string, key itemKey) (int, bool) {
	seen := 0
	for i, s := range items {
		if s != key.text {
			continue
		}
		if seen == key.nth {
			return i, true
		}
		seen++
	}
	return -1, false
}
