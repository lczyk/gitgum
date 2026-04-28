package fuzzyfinder

import (
	"testing"

	"github.com/lczyk/assert"
)

func TestKeyOf_SimpleString(t *testing.T) {
	items := []string{"a", "b", "c"}
	assert.Equal(t, keyOf(items, 0), itemKey{text: "a", nth: 0})
	assert.Equal(t, keyOf(items, 1), itemKey{text: "b", nth: 0})
	assert.Equal(t, keyOf(items, 2), itemKey{text: "c", nth: 0})
}

func TestKeyOf_Duplicates(t *testing.T) {
	items := []string{"a", "b", "a", "a", "b"}
	assert.Equal(t, keyOf(items, 0), itemKey{text: "a", nth: 0})
	assert.Equal(t, keyOf(items, 1), itemKey{text: "b", nth: 0})
	assert.Equal(t, keyOf(items, 2), itemKey{text: "a", nth: 1})
	assert.Equal(t, keyOf(items, 3), itemKey{text: "a", nth: 2})
	assert.Equal(t, keyOf(items, 4), itemKey{text: "b", nth: 1})
}

func TestKeyOf_OutOfRange(t *testing.T) {
	items := []string{"a"}
	assert.Equal(t, keyOf(items, -1), itemKey{})
	assert.Equal(t, keyOf(items, 5), itemKey{})
}

func TestFindKey_Hit(t *testing.T) {
	items := []string{"a", "b", "a", "c"}
	idx, ok := findKey(items, itemKey{text: "a", nth: 1})
	assert.That(t, ok, "second 'a' should be found")
	assert.Equal(t, idx, 2)

	idx, ok = findKey(items, itemKey{text: "c", nth: 0})
	assert.That(t, ok, "single 'c' should be found")
	assert.Equal(t, idx, 3)
}

func TestFindKey_Miss(t *testing.T) {
	items := []string{"a", "b"}
	_, ok := findKey(items, itemKey{text: "missing", nth: 0})
	assert.That(t, !ok, "missing text should not be found")

	_, ok = findKey(items, itemKey{text: "a", nth: 1})
	assert.That(t, !ok, "second 'a' doesn't exist")
}

// TestKeyRoundTrip: every item's key resolves back to its original index.
func TestKeyRoundTrip(t *testing.T) {
	items := []string{"x", "x", "y", "x", "y", "z"}
	for i := range items {
		k := keyOf(items, i)
		idx, ok := findKey(items, k)
		assert.That(t, ok, "key for items[%d] not found", i)
		assert.Equal(t, idx, i)
	}
}

// TestKeyRoundTrip_AfterRemoval: when we remove the first 'x', the key for
// the second-original 'x' (which had nth=1 in the old slice) maps to nth=0
// in the new slice — so the caller must recompute keys before remapping
// across slices. This test documents the helper's contract: keys are valid
// *within* a snapshot and must be re-derived against the new snapshot.
func TestKeyRoundTrip_AfterRemoval(t *testing.T) {
	old := []string{"x", "x", "y"}
	keyOldSecondX := keyOf(old, 1)
	assert.Equal(t, keyOldSecondX, itemKey{text: "x", nth: 1})

	// Caller removes old[0]. New slice = ["x", "y"].
	new := []string{"x", "y"}

	// Looking up the old key in the new slice fails — there's no second 'x'.
	_, ok := findKey(new, keyOldSecondX)
	assert.That(t, !ok, "old key for second 'x' shouldn't survive removal of first 'x'")

	// What survives is the *position-aware* remap done by the picker: it
	// looks up by (text, nth_among_survivors). After removing the first 'x',
	// the surviving 'x' is at nth=0.
	survivorKey := itemKey{text: "x", nth: 0}
	idx, ok := findKey(new, survivorKey)
	assert.That(t, ok, "surviving 'x' should be found at nth=0")
	assert.Equal(t, idx, 0)
}
