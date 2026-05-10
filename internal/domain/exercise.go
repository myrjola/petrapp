// Package domain holds the pure entities, value objects, aggregate methods,
// and domain services for the workout bounded context. It depends on the Go
// standard library only — no SQL, no HTTP, no logger, no third-party clients.
//
// Domain code is the canonical home for business rules. Other layers
// (repository, service, handlers) build on top of these types.
package domain

import (
	"fmt"
	"strconv"
)

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

// Exercise represents a single exercise type, e.g. Squat, Bench Press, etc.
type Exercise struct {
	ID                     int          `json:"id"`
	Name                   string       `json:"name"`
	Category               Category     `json:"category"`
	ExerciseType           ExerciseType `json:"exercise_type"`
	DescriptionMarkdown    string       `json:"description_markdown"`
	PrimaryMuscleGroups    []string     `json:"primary_muscle_groups"`
	SecondaryMuscleGroups  []string     `json:"secondary_muscle_groups"`
	DefaultStartingSeconds *int         `json:"default_starting_seconds,omitempty"`
	RepMin                 *int         `json:"rep_min,omitempty"`
	RepMax                 *int         `json:"rep_max,omitempty"`
}

// IsTimed returns true if this exercise uses duration targets instead of rep counts.
func (e Exercise) IsTimed() bool { return e.ExerciseType == ExerciseTypeTime }

// HasWeight reports whether sets of this exercise carry a weight value.
// True for weighted and assisted exercises; false for bodyweight and
// time-based. Planning, set seeding, and the per-set form all branch on
// this — keeping the rule on the type prevents drift when a new
// ExerciseType is added.
func (e Exercise) HasWeight() bool {
	return e.ExerciseType == ExerciseTypeWeighted || e.ExerciseType == ExerciseTypeAssisted
}

// FormatSetValue returns the user-visible string for a set's target or
// completed value. Reps render as "%d"; seconds render as "%ds". The unit
// choice is driven by ExerciseType — display layers must call this rather
// than reconstruct the formatting from periodization or any other field.
func (e Exercise) FormatSetValue(value int) string {
	if e.IsTimed() {
		return fmt.Sprintf("%ds", value)
	}
	return strconv.Itoa(value)
}

// SetValueUnit returns the input-label unit for a set value: "reps" or
// "seconds". Used by handlers when rendering input form labels.
func (e Exercise) SetValueUnit() string {
	if e.IsTimed() {
		return "seconds"
	}
	return "reps"
}
