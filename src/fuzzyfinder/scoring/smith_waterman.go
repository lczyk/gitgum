package scoring

import "unicode"

// smithWaterman computes a local alignment score using affine gap penalty:
// openGap charged once per gap, extGap per character within it.
// Uses Gotoh's DP to stay O(MN) instead of O(M²N).
func smithWaterman(s1, s2 []rune) (int, [2]int) {
	if len(s1) == 0 {
		// s2 is also empty — caller enforces len(s1) >= len(s2)
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

	// if the best DP cell covered all of s2 (maxJ is the last s2 index), the match
	// ends at maxI; otherwise scan forward for the remaining s2 chars.
	if maxJ == len(s2)-1 {
		to = maxI
	} else {
		j := maxJ + 1
		for i := maxI + 1; i < len(s1); i++ {
			if unicode.ToLower(s1[i]) == unicode.ToLower(s2[j]) {
				j++
				if j == len(s2) {
					to = i
					break
				}
			}
		}
	}

	// scan left from maxI-1 matching s2[maxJ-1] down to s2[0].
	// from stays 0 (default) when s1[0] is the final match: the loop exits
	// without a subsequent iteration to detect nextS2 < 0.
	nextS2 := maxJ - 1
	for i := maxI - 1; i >= 0; i-- {
		if nextS2 < 0 {
			from = i + 1
			break
		}
		if unicode.ToLower(s1[i]) == unicode.ToLower(s2[nextS2]) {
			nextS2--
		}
	}

	// normalise: longer corpus penalises same raw alignment — effective score = maxScore² / len(s1)
	adjusted := float32(maxScore) * (float32(maxScore) / float32(len(s1)))
	return int(adjusted), [2]int{from, to}
}

func isDelimiter(r rune) bool {
	switch r {
	case '(', '[', '{', '/', '-', '_', '.':
		return true
	}
	return unicode.IsSpace(r)
}
