// Package domain holds the pure entities, value objects, aggregate methods,
// and domain services for the workout bounded context. It depends on the Go
// standard library only — no SQL, no HTTP, no logger, no third-party clients.
//
// Domain code is the canonical home for business rules. Other layers
// (repository, service, handlers) build on top of these types.
package domain

import (
	"fmt"
	"math"
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

// IsValid reports whether c is one of the defined Category values.
func (c Category) IsValid() bool {
	switch c {
	case CategoryFullBody, CategoryUpper, CategoryLower:
		return true
	default:
		return false
	}
}

// Label returns the human-readable workout-split name for display.
func (c Category) Label() string {
	switch c {
	case CategoryUpper:
		return "Upper Body"
	case CategoryLower:
		return "Lower Body"
	case CategoryFullBody:
		return "Full Body"
	default:
		return "Full Body"
	}
}

// ExerciseType distinguishes the load model used by an exercise.
type ExerciseType string

const (
	ExerciseTypeWeighted   ExerciseType = "weighted"
	ExerciseTypeBodyweight ExerciseType = "bodyweight"
	ExerciseTypeAssisted   ExerciseType = "assisted"
	ExerciseTypeTime       ExerciseType = "time_based"
)

// LoadModel is the axis that determines how a set is measured, progressed,
// and recorded. Several ExerciseTypes can share a LoadModel (weighted and
// assisted both load by weight); the workout handler branches on this rather
// than on ExerciseType, so a new type that reuses an existing load model
// needs no handler change.
type LoadModel int

const (
	LoadUnknown    LoadModel = iota // Zero value: unregistered type; all behavior degrades to a no-op.
	LoadWeighted                    // Carries a weight target; driven by the weight progression engine.
	LoadBodyweight                  // Reps only; the stored target is used as-is.
	LoadTimed                       // Duration target; driven by the timed progression engine.
)

// exerciseBehavior captures the per-ExerciseType rules the rest of the domain
// and the display layer branch on. Adding an ExerciseType means adding one
// entry to exerciseBehaviors; the exhaustiveness test in exercise_test.go
// fails until it exists. This table replaces the hand-synced switch statements
// these methods previously carried.
type exerciseBehavior struct {
	load         LoadModel        // How the set is loaded and measured.
	assistedSign bool             // Negate the weight magnitude when the assisted flag is set.
	formatSet    func(Set) string // History-line renderer; "" when the set lacks the values its type needs.
}

var exerciseBehaviors = map[ExerciseType]exerciseBehavior{ //nolint:gochecknoglobals // immutable lookup table
	ExerciseTypeWeighted:   {load: LoadWeighted, assistedSign: false, formatSet: formatWeightedSet},
	ExerciseTypeAssisted:   {load: LoadWeighted, assistedSign: true, formatSet: formatWeightedSet},
	ExerciseTypeBodyweight: {load: LoadBodyweight, assistedSign: false, formatSet: formatBodyweightSet},
	ExerciseTypeTime:       {load: LoadTimed, assistedSign: false, formatSet: formatTimedSet},
}

func formatWeightedSet(set Set) string {
	if set.WeightKg == nil || set.CompletedValue == nil {
		return ""
	}
	return fmt.Sprintf("%dx%.1fkg", *set.CompletedValue, *set.WeightKg)
}

func formatBodyweightSet(set Set) string {
	if set.CompletedValue == nil {
		return ""
	}
	return fmt.Sprintf("%d reps", *set.CompletedValue)
}

func formatTimedSet(set Set) string {
	if set.CompletedValue == nil {
		return ""
	}
	return fmt.Sprintf("%ds", *set.CompletedValue)
}

// LoadModel reports how this exercise's sets are loaded and measured.
func (e Exercise) LoadModel() LoadModel { return e.behavior().load }

// IsValid reports whether et is one of the defined ExerciseType values.
func (et ExerciseType) IsValid() bool {
	_, ok := exerciseBehaviors[et]
	return ok
}

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
func (e Exercise) IsTimed() bool { return e.behavior().load == LoadTimed }

// HasWeight reports whether sets of this exercise carry a weight value.
// True for weighted and assisted exercises; false for bodyweight and
// time-based. Planning, set seeding, and the per-set form all branch on
// this — keeping the rule on the type prevents drift when a new
// ExerciseType is added.
func (e Exercise) HasWeight() bool { return e.behavior().load == LoadWeighted }

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

// TargetRangeText renders the planned per-set target for sub-line display
// on the workout overview: "8–12 reps" for weighted/bodyweight/assisted (when
// RepMin/RepMax are set), "30s" for time-based (when DefaultStartingSeconds is
// set), or "" when the data is missing so callers can drop the line.
func (e Exercise) TargetRangeText() string {
	if e.IsTimed() {
		if e.DefaultStartingSeconds == nil {
			return ""
		}
		return fmt.Sprintf("%ds", *e.DefaultStartingSeconds)
	}
	if e.RepMin == nil || e.RepMax == nil {
		return ""
	}
	if *e.RepMin == *e.RepMax {
		return fmt.Sprintf("%d reps", *e.RepMin)
	}
	return fmt.Sprintf("%d–%d reps", *e.RepMin, *e.RepMax)
}

// EncodeFormWeight applies the assisted-exercise sign convention to a weight
// value parsed from the per-set form. For ExerciseTypeAssisted with the
// "assisted" flag set, the stored value is the negative magnitude of the
// input. All other types/flag combinations pass the input through unchanged.
func (e Exercise) EncodeFormWeight(input float64, assisted bool) float64 {
	if e.behavior().assistedSign && assisted {
		return -math.Abs(input)
	}
	return input
}

// FormatSetDescription renders a completed set as a single line for history
// display: "8x10.0kg" for weighted/assisted, "12 reps" for bodyweight, "30s"
// for time_based. Returns "" when the set is missing the values needed for
// its exercise type, so callers can drop empty entries.
func (e Exercise) FormatSetDescription(set Set) string {
	if fn := e.behavior().formatSet; fn != nil {
		return fn(set)
	}
	return "" // Unregistered type degrades gracefully, matching the old default case.
}

// Validate reports whether the exercise's fields form a persistable record.
// It returns a ValidationError carrying a user-facing message on the first
// rule it fails, and nil when every rule passes. It is the single source of
// truth for exercise-form validation; handlers detect the ValidationError
// with errors.As and surface it via the flash + banner flow. Validate checks
// only that the populated fields are valid and that the fields required for
// the exercise type are present — it does not cross-check that a timed
// exercise lacks a rep window, because handler struct-shaping guarantees it.
func (e Exercise) Validate() error {
	const (
		repBoundMin = 1
		repBoundMax = 50
	)
	if e.Name == "" {
		return ValidationError{Message: "Name is required."}
	}
	if !e.Category.IsValid() {
		return ValidationError{Message: "Category must be one of full body, upper, or lower."}
	}
	if !e.ExerciseType.IsValid() {
		return ValidationError{Message: "Exercise type must be weighted, bodyweight, assisted, or time_based."}
	}
	if e.IsTimed() && (e.DefaultStartingSeconds == nil || *e.DefaultStartingSeconds <= 0) {
		return ValidationError{
			Message: "Default starting seconds must be a positive integer for time-based exercises.",
		}
	}
	if len(e.PrimaryMuscleGroups) == 0 {
		return ValidationError{Message: "At least one primary muscle group is required."}
	}
	if !e.IsTimed() {
		if e.RepMin == nil || e.RepMax == nil ||
			*e.RepMin < repBoundMin || *e.RepMin > repBoundMax ||
			*e.RepMax < repBoundMin || *e.RepMax > repBoundMax {
			return ValidationError{Message: "Min and max reps must be whole numbers between 1 and 50."}
		}
		if *e.RepMin > *e.RepMax {
			return ValidationError{Message: "Min reps must be less than or equal to max reps."}
		}
	}
	return nil
}

// behavior returns the registered rules for this exercise's type, or the
// zero exerciseBehavior (LoadUnknown, nil formatter) for an unregistered
// type so callers degrade gracefully.
func (e Exercise) behavior() exerciseBehavior { return exerciseBehaviors[e.ExerciseType] }
