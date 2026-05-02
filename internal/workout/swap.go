package workout

// SwapSimilarityScore returns a non-negative integer describing how similar
// candidate is to current for the purposes of swapping one workout exercise
// for another. Higher means a better candidate.
//
// Weights:
//   - primary ∩ primary:     +4 per shared muscle
//   - primary ∩ secondary:   +2 per shared muscle (counted in both directions)
//   - secondary ∩ secondary: +1 per shared muscle
//   - same category:         +3 flat bonus
//
// The function is pure and symmetric: SwapSimilarityScore(a, b) ==
// SwapSimilarityScore(b, a).
func SwapSimilarityScore(current, candidate Exercise) int {
	const (
		primaryWeight          = 4
		primarySecondaryWeight = 2
		secondaryWeight        = 1
		categoryBonus          = 3
	)

	score := 0
	score += primaryWeight * countShared(current.PrimaryMuscleGroups, candidate.PrimaryMuscleGroups)
	score += primarySecondaryWeight * countShared(current.PrimaryMuscleGroups, candidate.SecondaryMuscleGroups)
	score += primarySecondaryWeight * countShared(current.SecondaryMuscleGroups, candidate.PrimaryMuscleGroups)
	score += secondaryWeight * countShared(current.SecondaryMuscleGroups, candidate.SecondaryMuscleGroups)
	if current.Category == candidate.Category {
		score += categoryBonus
	}
	return score
}

// countShared returns the number of strings appearing in both a and b.
// Inputs are treated as sets — duplicates within a single slice are not
// double-counted.
func countShared(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, m := range a {
		set[m] = struct{}{}
	}
	n := 0
	seen := make(map[string]struct{}, len(b))
	for _, m := range b {
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		if _, ok := set[m]; ok {
			n++
		}
	}
	return n
}
