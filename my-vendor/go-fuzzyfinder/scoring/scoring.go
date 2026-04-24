// Package scoring provides APIs that calculates similarity scores between two strings.
package scoring

import "fmt"

// Calculate calculates a similarity score between s1 and s2.
// s1 must have at least as many runes as s2.
func Calculate(s1, s2 string) (int, [2]int, error) {
	r1, r2 := []rune(s1), []rune(s2)
	if len(r1) < len(r2) {
		return 0, [2]int{}, fmt.Errorf("scoring: s1 must have at least as many runes as s2 (%d < %d)", len(r1), len(r2))
	}
	score, pos := smithWaterman(r1, r2)
	return score, pos, nil
}
