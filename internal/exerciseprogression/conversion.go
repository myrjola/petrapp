package exerciseprogression

// ConvertWeight translates a load chosen for fromReps into an equivalent load
// for toReps at the same estimated one-rep max, rounded to the nearest 0.5 kg.
// It uses the Epley formula: 1RM = w * (1 + r/30).
//
// Returns weight unchanged when fromReps == toReps, or when any input is
// non-positive. Non-positive weights cover assisted exercises (negative
// weight_kg representing assistance), where there is no meaningful 1RM
// analog — the assistance value is preserved verbatim across periodizations.
func ConvertWeight(weight float64, fromReps, toReps int) float64 {
	if weight <= 0 || fromReps <= 0 || toReps <= 0 || fromReps == toReps {
		return weight
	}
	oneRepMax := weight * (1 + float64(fromReps)/30)
	converted := oneRepMax / (1 + float64(toReps)/30)
	return roundToHalf(converted)
}
