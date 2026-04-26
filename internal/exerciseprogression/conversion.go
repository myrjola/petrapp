package exerciseprogression

// ConvertWeight translates a load chosen for fromReps into an equivalent load
// for toReps at the same estimated one-rep max, rounded to the nearest 0.5 kg.
// It uses the Epley formula: 1RM = w * (1 + r/30).
//
// Returns weight unchanged when fromReps == toReps, or when any input is
// non-positive.
func ConvertWeight(weight float64, fromReps, toReps int) float64 {
	if weight <= 0 || fromReps <= 0 || toReps <= 0 || fromReps == toReps {
		return weight
	}
	oneRepMax := weight * (1 + float64(fromReps)/30)
	converted := oneRepMax / (1 + float64(toReps)/30)
	return roundToHalf(converted)
}
