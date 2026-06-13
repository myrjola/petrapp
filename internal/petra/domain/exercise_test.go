package domain_test

import (
	"errors"
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func Test_Exercise_FormatSetValue(t *testing.T) {
	t.Parallel()

	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		value    int
		want     string
	}{
		{"weighted formats as integer", mkExercise(domain.ExerciseTypeWeighted), 8, "8"},
		{"bodyweight formats as integer", mkExercise(domain.ExerciseTypeBodyweight), 12, "12"},
		{"assisted formats as integer", mkExercise(domain.ExerciseTypeAssisted), 5, "5"},
		{"time_based formats as seconds", mkExercise(domain.ExerciseTypeTime), 30, "30s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.exercise.FormatSetValue(tc.value)
			if got != tc.want {
				t.Errorf("Exercise{%s}.FormatSetValue(%d) = %q, want %q",
					tc.exercise.ExerciseType, tc.value, got, tc.want)
			}
		})
	}
}

func Test_Exercise_HasWeight(t *testing.T) {
	t.Parallel()

	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		want     bool
	}{
		{"weighted has weight", mkExercise(domain.ExerciseTypeWeighted), true},
		{"assisted has weight", mkExercise(domain.ExerciseTypeAssisted), true},
		{"bodyweight has no weight", mkExercise(domain.ExerciseTypeBodyweight), false},
		{"time_based has no weight", mkExercise(domain.ExerciseTypeTime), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.exercise.HasWeight()
			if got != tc.want {
				t.Errorf("Exercise{%s}.HasWeight() = %v, want %v",
					tc.exercise.ExerciseType, got, tc.want)
			}
		})
	}
}

func Test_Exercise_LoadModel(t *testing.T) {
	t.Parallel()

	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		want     domain.LoadModel
	}{
		{"weighted loads by weight", mkExercise(domain.ExerciseTypeWeighted), domain.LoadWeighted},
		{"assisted loads by weight", mkExercise(domain.ExerciseTypeAssisted), domain.LoadWeighted},
		{"bodyweight loads by bodyweight", mkExercise(domain.ExerciseTypeBodyweight), domain.LoadBodyweight},
		{"time_based loads by time", mkExercise(domain.ExerciseTypeTime), domain.LoadTimed},
		{"unknown loads as unknown", mkExercise(domain.ExerciseType("garbage")), domain.LoadUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.exercise.LoadModel()
			if got != tc.want {
				t.Errorf("Exercise{%s}.LoadModel() = %v, want %v",
					tc.exercise.ExerciseType, got, tc.want)
			}
		})
	}
}

// Test_ExerciseType_behaviorExhaustive guards the single registration point:
// every ExerciseType in allExerciseTypes must resolve to a real LoadModel.
// A new ExerciseType const added without a behavior entry fails here.
func Test_ExerciseType_behaviorExhaustive(t *testing.T) {
	t.Parallel()

	allExerciseTypes := []domain.ExerciseType{
		domain.ExerciseTypeWeighted,
		domain.ExerciseTypeBodyweight,
		domain.ExerciseTypeAssisted,
		domain.ExerciseTypeTime,
	}

	for _, et := range allExerciseTypes {
		if !et.IsValid() {
			t.Errorf("ExerciseType %q has no behavior entry", et)
		}
		ex := domain.Exercise{ExerciseType: et} //nolint:exhaustruct // Only ExerciseType is read.
		if ex.LoadModel() == domain.LoadUnknown {
			t.Errorf("ExerciseType %q resolves to LoadUnknown; add a behavior entry", et)
		}
	}
}

func Test_Category_IsValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		c    domain.Category
		want bool
	}{
		{"full_body", domain.CategoryFullBody, true},
		{"upper", domain.CategoryUpper, true},
		{"lower", domain.CategoryLower, true},
		{"empty", domain.Category(""), false},
		{"unknown", domain.Category("garbage"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.c.IsValid(); got != tc.want {
				t.Errorf("Category(%q).IsValid() = %v, want %v", tc.c, got, tc.want)
			}
		})
	}
}

func Test_ExerciseType_IsValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		et   domain.ExerciseType
		want bool
	}{
		{"weighted", domain.ExerciseTypeWeighted, true},
		{"bodyweight", domain.ExerciseTypeBodyweight, true},
		{"assisted", domain.ExerciseTypeAssisted, true},
		{"time_based", domain.ExerciseTypeTime, true},
		{"empty", domain.ExerciseType(""), false},
		{"unknown", domain.ExerciseType("garbage"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.et.IsValid(); got != tc.want {
				t.Errorf("ExerciseType(%q).IsValid() = %v, want %v", tc.et, got, tc.want)
			}
		})
	}
}

func Test_Exercise_EncodeFormWeight(t *testing.T) {
	t.Parallel()

	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		input    float64
		assisted bool
		want     float64
	}{
		{"weighted ignores assisted flag", mkExercise(domain.ExerciseTypeWeighted), 50, true, 50},
		{"weighted unchecked", mkExercise(domain.ExerciseTypeWeighted), 50, false, 50},
		{"assisted with flag negates", mkExercise(domain.ExerciseTypeAssisted), 20, true, -20},
		{"assisted without flag keeps input", mkExercise(domain.ExerciseTypeAssisted), 20, false, 20},
		{"assisted with flag is idempotent on negative input", mkExercise(domain.ExerciseTypeAssisted), -20, true, -20},
		{"bodyweight ignores flag", mkExercise(domain.ExerciseTypeBodyweight), 0, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.exercise.EncodeFormWeight(tc.input, tc.assisted)
			if got != tc.want {
				t.Errorf("Exercise{%s}.EncodeFormWeight(%v, %v) = %v, want %v",
					tc.exercise.ExerciseType, tc.input, tc.assisted, got, tc.want)
			}
		})
	}
}

func Test_Exercise_FormatSetDescription(t *testing.T) {
	t.Parallel()

	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}
	weight := func(v float64) *float64 { return &v }
	completed := func(v int) *int { return &v }
	mkSet := func(w *float64, c *int) domain.Set {
		return domain.Set{
			WeightKg:       w,
			TargetValue:    0,
			CompletedValue: c,
			CompletedAt:    nil,
			Signal:         nil,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		set      domain.Set
		want     string
	}{
		{"weighted with weight and reps", mkExercise(domain.ExerciseTypeWeighted),
			mkSet(weight(10), completed(8)), "8x10.0kg"},
		{"weighted missing weight", mkExercise(domain.ExerciseTypeWeighted),
			mkSet(nil, completed(8)), ""},
		{"weighted missing completed", mkExercise(domain.ExerciseTypeWeighted),
			mkSet(weight(10), nil), ""},
		{"assisted preserves negative weight", mkExercise(domain.ExerciseTypeAssisted),
			mkSet(weight(-5), completed(12)), "12x-5.0kg"},
		{"bodyweight reps", mkExercise(domain.ExerciseTypeBodyweight),
			mkSet(nil, completed(15)), "15 reps"},
		{"bodyweight missing completed", mkExercise(domain.ExerciseTypeBodyweight),
			mkSet(nil, nil), ""},
		{"time_based seconds", mkExercise(domain.ExerciseTypeTime),
			mkSet(nil, completed(30)), "30s"},
		{"time_based missing completed", mkExercise(domain.ExerciseTypeTime),
			mkSet(nil, nil), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.exercise.FormatSetDescription(tc.set)
			if got != tc.want {
				t.Errorf("Exercise{%s}.FormatSetDescription(...) = %q, want %q",
					tc.exercise.ExerciseType, got, tc.want)
			}
		})
	}
}

func Test_Category_Label(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		category domain.Category
		want     string
	}{
		{"upper", domain.CategoryUpper, "Upper Body"},
		{"lower", domain.CategoryLower, "Lower Body"},
		{"full body", domain.CategoryFullBody, "Full Body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.category.Label(); got != tc.want {
				t.Errorf("Label() = %q, want %q", got, tc.want)
			}
		})
	}
}

func Test_SetTarget_AbsWeightKg(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		t    domain.SetTarget
		want float64
	}{
		{"positive weight", domain.SetTarget{WeightKg: 50, TargetValue: 0}, 50},
		{"negative weight (assisted convention)", domain.SetTarget{WeightKg: -10, TargetValue: 0}, 10},
		{"zero weight", domain.SetTarget{WeightKg: 0, TargetValue: 0}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.t.AbsWeightKg(); got != tc.want {
				t.Errorf("SetTarget{%v}.AbsWeightKg() = %v, want %v", tc.t.WeightKg, got, tc.want)
			}
		})
	}
}

func Test_Exercise_SetValueUnit(t *testing.T) {
	t.Parallel()

	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		want     string
	}{
		{"weighted is reps", mkExercise(domain.ExerciseTypeWeighted), "reps"},
		{"bodyweight is reps", mkExercise(domain.ExerciseTypeBodyweight), "reps"},
		{"assisted is reps", mkExercise(domain.ExerciseTypeAssisted), "reps"},
		{"time_based is seconds", mkExercise(domain.ExerciseTypeTime), "seconds"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.exercise.SetValueUnit()
			if got != tc.want {
				t.Errorf("Exercise{%s}.SetValueUnit() = %q, want %q",
					tc.exercise.ExerciseType, got, tc.want)
			}
		})
	}
}

func Test_Exercise_Validate(t *testing.T) {
	t.Parallel()

	intPtr := func(n int) *int { return &n }
	validWeighted := func() domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // test builder sets only the validated fields.
			Name:                "Bench Press",
			Category:            domain.CategoryUpper,
			ExerciseType:        domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"},
			RepMin:              intPtr(5),
			RepMax:              intPtr(10),
		}
	}
	validTimed := func() domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // test builder sets only the validated fields.
			Name:                   "Plank",
			Category:               domain.CategoryFullBody,
			ExerciseType:           domain.ExerciseTypeTime,
			PrimaryMuscleGroups:    []string{"Core"},
			DefaultStartingSeconds: intPtr(30),
		}
	}

	cases := []struct {
		name        string
		exercise    domain.Exercise
		wantErr     bool
		wantMessage string
	}{
		{"valid weighted", validWeighted(), false, ""},
		{"valid timed", validTimed(), false, ""},
		{
			"empty name",
			func() domain.Exercise { e := validWeighted(); e.Name = ""; return e }(),
			true, "Name is required.",
		},
		{
			"invalid category",
			func() domain.Exercise { e := validWeighted(); e.Category = domain.Category("bogus"); return e }(),
			true, "Category must be one of full body, upper, or lower.",
		},
		{
			"invalid type",
			func() domain.Exercise { e := validWeighted(); e.ExerciseType = domain.ExerciseType("bogus"); return e }(),
			true, "Exercise type must be weighted, bodyweight, assisted, or time_based.",
		},
		{
			"timed without seconds",
			func() domain.Exercise { e := validTimed(); e.DefaultStartingSeconds = nil; return e }(),
			true, "Default starting seconds must be a positive integer for time-based exercises.",
		},
		{
			"timed with zero seconds",
			func() domain.Exercise { e := validTimed(); e.DefaultStartingSeconds = intPtr(0); return e }(),
			true, "Default starting seconds must be a positive integer for time-based exercises.",
		},
		{
			"no primary muscles",
			func() domain.Exercise { e := validWeighted(); e.PrimaryMuscleGroups = nil; return e }(),
			true, "At least one primary muscle group is required.",
		},
		{
			"missing rep range",
			func() domain.Exercise { e := validWeighted(); e.RepMin = nil; e.RepMax = nil; return e }(),
			true, "Min and max reps must be whole numbers between 1 and 50.",
		},
		{
			"rep range out of bounds",
			func() domain.Exercise { e := validWeighted(); e.RepMax = intPtr(99); return e }(),
			true, "Min and max reps must be whole numbers between 1 and 50.",
		},
		{
			"rep min greater than max",
			func() domain.Exercise { e := validWeighted(); e.RepMin = intPtr(12); e.RepMax = intPtr(8); return e }(),
			true, "Min reps must be less than or equal to max reps.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.exercise.Validate()
			if !tc.wantErr {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error %q", tc.wantMessage)
			}
			var ve domain.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Validate() error is not a ValidationError: %v", err)
			}
			if ve.Message != tc.wantMessage {
				t.Errorf("Validate() message = %q, want %q", ve.Message, tc.wantMessage)
			}
		})
	}
}
