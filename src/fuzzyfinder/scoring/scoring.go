// Package scoring provides an API for calculating similarity scores between strings.
package scoring

import "fmt"

// Calculate returns a similarity score for s1 against s2.
// s1 is the corpus, s2 is the query; s1 must have at least as many runes as s2.
func Calculate(s1, s2 string) (int, [2]int, error) {
	r1, r2 := []rune(s1), []rune(s2)
	if len(r1) < len(r2) {
		return 0, [2]int{}, fmt.Errorf("scoring: s1 must have at least as many runes as s2 (%d < %d)", len(r1), len(r2))
	}
	score, pos := smithWaterman(r1, r2)
	return score, pos, nil
}
