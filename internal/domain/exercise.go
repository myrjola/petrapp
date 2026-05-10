// Package domain holds the pure entities, value objects, aggregate methods,
// and domain services for the workout bounded context. It depends on the Go
// standard library only — no SQL, no HTTP, no logger, no third-party clients.
//
// Domain code is the canonical home for business rules. Other layers
// (repository, service, handlers) build on top of these types.
package domain

// Category is the workout focus for a session — the muscle-group split a day
// targets.
type Category string

const (
	CategoryFullBody Category = "full_body"
	CategoryUpper    Category = "upper"
	CategoryLower    Category = "lower"
)

// ExerciseType distinguishes the load model used by an exercise.
type ExerciseType string

const (
	ExerciseTypeWeighted   ExerciseType = "weighted"
	ExerciseTypeBodyweight ExerciseType = "bodyweight"
	ExerciseTypeAssisted   ExerciseType = "assisted"
	ExerciseTypeTime       ExerciseType = "time_based"
)

// Resource is a learning link associated with an exercise (video, article).
type Resource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}
