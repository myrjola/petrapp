package exerciseprogression

// ConvertWeight translates a load chosen for fromReps into an equivalent load
// for toReps at the same estimated one-rep max, snapped to the nearest
// realisable load (1kg in the dumbbell range, 0.5kg above). It uses the
// Epley formula: 1RM = w * (1 + r/30).
//
// For positive weights, more reps map to a lighter load (e.g. 100 kg x5 →
// ~92 kg x8). For negative weights — the assisted-exercise convention where
// weight_kg is the magnitude of machine assistance — the scaling is inverted:
// more reps require more assistance, so the magnitude grows (e.g. -50 kg x5
// → ~-54.5 kg x8). Without a tracked bodyweight we cannot compute the true
// effective load, but inverting the Epley ratio keeps the directional change
// consistent with the within-session TooHeavy/TooLight behaviour.
//
// Returns weight unchanged when fromReps == toReps, when fromReps or toReps
// are non-positive, or when weight is exactly zero (no scaling reference).
func ConvertWeight(weight float64, fromReps, toReps int) float64 {
	if weight == 0 || fromReps <= 0 || toReps <= 0 || fromReps == toReps {
		return weight
	}
	if weight < 0 {
		converted := weight * (1 + float64(toReps)/30) / (1 + float64(fromReps)/30)
		return snapWeight(converted)
	}
	oneRepMax := weight * (1 + float64(fromReps)/30)
	converted := oneRepMax / (1 + float64(toReps)/30)
	return snapWeight(converted)
}
