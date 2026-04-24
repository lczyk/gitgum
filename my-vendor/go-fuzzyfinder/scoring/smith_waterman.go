package scoring

import "unicode"

// smithWaterman calculates a similarity score between s1 and s2
// by smith-waterman algorithm. smith-waterman algorithm is one of
// local alignment algorithms and it uses dynamic programming.
//
// In this smith-waterman algorithm, we use the affine gap penalty.
// Please see https://en.wikipedia.org/wiki/Gap_penalty#Affine for additional
// information about the affine gap penalty.
//
// We calculate the gap penalty by the Gotoh's algorithm, which optimizes
// the calculation from O(M^2N) to O(MN).
// Please see ftp://150.128.97.71/pub/Bioinformatica/gotoh1982.pdf for more details.
func smithWaterman(s1, s2 []rune) (int, [2]int) {
	if len(s1) == 0 {
		// If the length of s1 is 0, also the length of s2 is 0.
		return 0, [2]int{-1, -1}
	}

	const (
		openGap int32 = 5 // Gap opening penalty.
		extGap  int32 = 1 // Gap extension penalty.

		matchScore    int32 = 5
		mismatchScore int32 = 1

		firstCharBonus int32 = 3 // The first char of s1 is equal to s2's one.
	)

	// H is the scoring matrix; D tracks gap penalties for s2 (no s1 gap matrix
	// needed because s1 contains all runes of s2 and is never gapped).
	H := make([][]int32, len(s1)+1)
	D := make([][]int32, len(s1)+1)
	for i := 0; i <= len(s1); i++ {
		H[i] = make([]int32, len(s2)+1)
		D[i] = make([]int32, len(s2)+1)
		D[i][0] = -openGap - int32(i)*extGap
	}

	// Calculate bonuses for each rune of s1. First rune always gets bonus;
	// subsequent runes get it when they immediately follow a delimiter (word-start).
	bonus := make([]int32, len(s1))
	bonus[0] = firstCharBonus
	for i := 1; i < len(s1); i++ {
		if isDelimiter(s1[i-1]) && !isDelimiter(s1[i]) {
			bonus[i] = firstCharBonus
		}
	}

	var maxScore int32
	var maxI int
	var maxJ int
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			var score int32
			if s1[i-1] != s2[j-1] {
				score = H[i-1][j-1] - mismatchScore
			} else {
				score = H[i-1][j-1] + matchScore + bonus[i-1]
			}
			H[i][j] = max(D[i-1][j], score, 0)

			D[i][j] = max(H[i-1][j]-openGap, D[i-1][j]-extGap)

			// Update the max score.
			// Don't pick a position that is less than the length of s2.
			if H[i][j] > maxScore && i >= j {
				maxScore = H[i][j]
				maxI = i - 1
				maxJ = j - 1
			}
		}
	}

	// Determine the matched position.

	var from, to int
	cnt := 1

	// maxJ is the last index of s2.
	// If maxJ is equal to the length of s2, it means there are no matched runes after maxJ.
	if maxJ == len(s2)-1 {
		to = maxI
	} else {
		j := maxJ + 1
		for i := maxI + 1; i < len(s1); i++ {
			if unicode.ToLower(s1[i]) == unicode.ToLower(s2[j]) {
				cnt++
				j++
				if j == len(s2) {
					to = i + 1
					break
				}
			}
		}
	}

	for i := maxI - 1; i > 0; i-- {
		if cnt == len(s2) {
			from = i + 1
			break
		}
		if unicode.ToLower(s1[i]) == unicode.ToLower(s2[len(s2)-1-cnt]) {
			cnt++
		}
	}

	// We adjust scores by the weight per one rune.
	return int(float32(maxScore) * (float32(maxScore) / float32(len(s1)))), [2]int{from, to}
}

var delimiterRunes = map[rune]struct{}{
	'(': {},
	'[': {},
	'{': {},
	'/': {},
	'-': {},
	'_': {},
	'.': {},
}

func isDelimiter(r rune) bool {
	_, ok := delimiterRunes[r]
	return ok || unicode.IsSpace(r)
}
