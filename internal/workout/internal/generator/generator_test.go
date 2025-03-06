package generator_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/ptr"
	"github.com/myrjola/petrapp/internal/workout"
	"github.com/myrjola/petrapp/internal/workout/internal/generator"
)

// TestGenerate verifies that the workout generator generates
// valid workouts according to our specifications.
func TestGenerate(t *testing.T) {
	// Define test cases with different input parameters
	testCases := []struct {
		name        string
		preferences workout.Preferences
		history     []workout.Session
		pool        []workout.Exercise
		date        time.Time
	}{
		{
			name: "New user with no history",
			preferences: workout.Preferences{
				Monday:    true,
				Wednesday: true,
				Friday:    true,
				Tuesday:   false,
				Thursday:  false,
				Saturday:  false,
				Sunday:    false,
			},
			history: nil,
			pool:    createExercisePool(),
			date:    time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC), // Monday
		},
		{
			name: "User with workout history",
			preferences: workout.Preferences{
				Monday:    true,
				Tuesday:   true,
				Thursday:  true,
				Saturday:  true,
				Wednesday: false,
				Friday:    false,
				Sunday:    false,
			},
			history: createWorkoutHistory(),
			pool:    createExercisePool(),
			date:    time.Date(2023, 1, 3, 0, 0, 0, 0, time.UTC), // Tuesday
		},
		{
			name: "Weekend workout",
			preferences: workout.Preferences{
				Saturday:  true,
				Sunday:    true,
				Monday:    false,
				Tuesday:   false,
				Wednesday: false,
				Thursday:  false,
				Friday:    false,
			},
			history: createWorkoutHistory(),
			pool:    createExercisePool(),
			date:    time.Date(2023, 1, 7, 0, 0, 0, 0, time.UTC), // Saturday
		},
	}

	// Run each test case
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create generator with test data
			gen := generator.NewGenerator(tc.preferences, tc.history, tc.pool)

			// Generate a workout
			session, err := gen.Generate(tc.date)
			if err != nil {
				t.Fatalf("Generate returned unexpected error: %v", err)
			}

			// Verify generated workout
			verifyGeneratedWorkout(t, session, tc.date)
		})
	}
}

// TestGenerateErrorHandling verifies that generator returns appropriate errors.
func TestGenerateErrorHandling(t *testing.T) {
	// Create a generator with no exercises in the pool
	emptyPreferences := workout.Preferences{
		Monday:    false,
		Tuesday:   false,
		Wednesday: false,
		Thursday:  false,
		Friday:    false,
		Saturday:  false,
		Sunday:    false,
	}
	emptyHistory := []workout.Session{}
	emptyPool := []workout.Exercise{}

	gen := generator.NewGenerator(emptyPreferences, emptyHistory, emptyPool)

	// Should return an error when there are no exercises available
	session, err := gen.Generate(time.Now())

	if err == nil {
		t.Error("Expected an error when generating a workout with an empty exercise pool, but got nil")
	}

	if session.WorkoutDate.IsZero() == false {
		t.Error("Expected empty session when error occurred, but got a populated session")
	}
}

// Helper function to verify a generated workout.
func verifyGeneratedWorkout(t *testing.T, session workout.Session, date time.Time) {
	t.Helper()

	// Verify date is correct
	if !session.WorkoutDate.Equal(date) {
		t.Errorf("Expected workout date %v, got %v", date, session.WorkoutDate)
	}

	// Verify status is planned
	if session.Status != workout.StatusPlanned {
		t.Errorf("Expected workout status to be %v, got %v", workout.StatusPlanned, session.Status)
	}

	// Verify workout has appropriate number of exercises (5-8)
	exerciseCount := len(session.ExerciseSets)
	if exerciseCount < 5 || exerciseCount > 8 {
		t.Errorf("Expected between 5-8 exercises, got %d", exerciseCount)
	}

	// Verify all exercises have appropriate number of sets and rep ranges
	for i, exerciseSet := range session.ExerciseSets {
		// Check if each exercise has a name
		if exerciseSet.Exercise.Name == "" {
			t.Errorf("Exercise #%d has no name", i+1)
		}

		// Check set count (3-6 sets per exercise)
		setCount := len(exerciseSet.Sets)
		if setCount < 3 || setCount > 6 {
			t.Errorf("Exercise %s has %d sets, expected 3-6 sets",
				exerciseSet.Exercise.Name, setCount)
		}

		// Verify set parameters
		for j, set := range exerciseSet.Sets {
			// Verify all weights are reasonable
			if set.WeightKg < 0 {
				t.Errorf("Exercise %s, set #%d has negative weight: %f",
					exerciseSet.Exercise.Name, j+1, set.WeightKg)
			}

			// Verify rep ranges (3-16)
			if set.MinReps < 3 || set.MaxReps > 16 {
				t.Errorf("Exercise %s, set #%d has unusual rep range: %d-%d",
					exerciseSet.Exercise.Name, j+1, set.MinReps, set.MaxReps)
			}

			// Verify min reps <= max reps
			if set.MinReps > set.MaxReps {
				t.Errorf("Exercise %s, set #%d has invalid rep range: min %d > max %d",
					exerciseSet.Exercise.Name, j+1, set.MinReps, set.MaxReps)
			}

			// Verify no completed reps in a new workout
			if set.CompletedReps != nil {
				t.Errorf("Exercise %s, set #%d has completed reps in a new workout",
					exerciseSet.Exercise.Name, j+1)
			}
		}

		// Verify set consistency (all sets for an exercise should have the same rep range)
		if !setsHaveConsistentRepRanges(exerciseSet.Sets) {
			t.Errorf("Exercise %s has inconsistent rep ranges across sets",
				exerciseSet.Exercise.Name)
		}
	}

	// Verify exercise uniqueness
	if !hasUniqueExercises(session.ExerciseSets) {
		t.Error("Workout contains duplicate exercises")
	}
}

// Helper function to check if an exercise set has consistent rep ranges.
func setsHaveConsistentRepRanges(sets []workout.Set) bool {
	if len(sets) == 0 {
		return true
	}

	minReps := sets[0].MinReps
	maxReps := sets[0].MaxReps

	for _, set := range sets {
		if set.MinReps != minReps || set.MaxReps != maxReps {
			return false
		}
	}

	return true
}

// Helper function to check if all exercises in a workout are unique.
func hasUniqueExercises(exerciseSets []workout.ExerciseSet) bool {
	exerciseIDs := make(map[int]bool)

	for _, es := range exerciseSets {
		if exerciseIDs[es.Exercise.ID] {
			return false
		}
		exerciseIDs[es.Exercise.ID] = true
	}

	return true
}

// Helper function to create a diverse pool of exercises.
func createExercisePool() []workout.Exercise {
	return []workout.Exercise{
		// Upper body exercises
		{
			ID:                    1,
			Name:                  "Bench Press",
			Category:              workout.CategoryUpper,
			PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
			SecondaryMuscleGroups: []string{"Shoulders"},
		},
		{
			ID:                    2,
			Name:                  "Pull Up",
			Category:              workout.CategoryUpper,
			PrimaryMuscleGroups:   []string{"Back", "Biceps"},
			SecondaryMuscleGroups: []string{"Forearms"},
		},
		{
			ID:                    3,
			Name:                  "Shoulder Press",
			Category:              workout.CategoryUpper,
			PrimaryMuscleGroups:   []string{"Shoulders"},
			SecondaryMuscleGroups: []string{"Triceps"},
		},
		{
			ID:                    4,
			Name:                  "Bicep Curl",
			Category:              workout.CategoryUpper,
			PrimaryMuscleGroups:   []string{"Biceps"},
			SecondaryMuscleGroups: []string{"Forearms"},
		},
		{
			ID:                    5,
			Name:                  "Tricep Extension",
			Category:              workout.CategoryUpper,
			PrimaryMuscleGroups:   []string{"Triceps"},
			SecondaryMuscleGroups: []string{},
		},
		{
			ID:                    6,
			Name:                  "Lateral Raise",
			Category:              workout.CategoryUpper,
			PrimaryMuscleGroups:   []string{"Shoulders"},
			SecondaryMuscleGroups: []string{},
		},

		// Lower body exercises
		{
			ID:                    7,
			Name:                  "Squat",
			Category:              workout.CategoryLower,
			PrimaryMuscleGroups:   []string{"Quadriceps", "Glutes"},
			SecondaryMuscleGroups: []string{"Hamstrings", "Core"},
		},
		{
			ID:                    8,
			Name:                  "Deadlift",
			Category:              workout.CategoryLower,
			PrimaryMuscleGroups:   []string{"Hamstrings", "Back"},
			SecondaryMuscleGroups: []string{"Glutes", "Forearms"},
		},
		{
			ID:                    9,
			Name:                  "Leg Press",
			Category:              workout.CategoryLower,
			PrimaryMuscleGroups:   []string{"Quadriceps"},
			SecondaryMuscleGroups: []string{"Glutes", "Hamstrings"},
		},
		{
			ID:                    10,
			Name:                  "Leg Curl",
			Category:              workout.CategoryLower,
			PrimaryMuscleGroups:   []string{"Hamstrings"},
			SecondaryMuscleGroups: []string{},
		},
		{
			ID:                    11,
			Name:                  "Calf Raise",
			Category:              workout.CategoryLower,
			PrimaryMuscleGroups:   []string{"Calves"},
			SecondaryMuscleGroups: []string{},
		},
		{
			ID:                    12,
			Name:                  "Leg Extension",
			Category:              workout.CategoryLower,
			PrimaryMuscleGroups:   []string{"Quadriceps"},
			SecondaryMuscleGroups: []string{},
		},

		// Full body exercises
		{
			ID:                    13,
			Name:                  "Clean and Press",
			Category:              workout.CategoryFullBody,
			PrimaryMuscleGroups:   []string{"Shoulders", "Legs", "Back"},
			SecondaryMuscleGroups: []string{"Core", "Arms"},
		},
		{
			ID:                    14,
			Name:                  "Burpee",
			Category:              workout.CategoryFullBody,
			PrimaryMuscleGroups:   []string{"Chest", "Legs", "Shoulders"},
			SecondaryMuscleGroups: []string{"Core", "Arms"},
		},
		{
			ID:                    15,
			Name:                  "Kettlebell Swing",
			Category:              workout.CategoryFullBody,
			PrimaryMuscleGroups:   []string{"Hamstrings", "Glutes", "Back"},
			SecondaryMuscleGroups: []string{"Shoulders", "Core"},
		},
		{
			ID:                    16,
			Name:                  "Turkish Get-Up",
			Category:              workout.CategoryFullBody,
			PrimaryMuscleGroups:   []string{"Shoulders", "Core", "Legs"},
			SecondaryMuscleGroups: []string{"Arms", "Back"},
		},
		{
			ID:                    17,
			Name:                  "Thruster",
			Category:              workout.CategoryFullBody,
			PrimaryMuscleGroups:   []string{"Legs", "Shoulders"},
			SecondaryMuscleGroups: []string{"Core", "Arms"},
		},
		{
			ID:                    18,
			Name:                  "Power Clean",
			Category:              workout.CategoryFullBody,
			PrimaryMuscleGroups:   []string{"Back", "Legs", "Shoulders"},
			SecondaryMuscleGroups: []string{"Arms", "Core"},
		},
	}
}

// Helper function to create mock workout history.
func createWorkoutHistory() []workout.Session {
	// Example completed rep counts
	rep8 := 8
	rep10 := 10
	rep12 := 12
	rep6 := 6

	// Create some sample exercises
	exercises := createExercisePool()

	// Set mock dates for the workouts
	twoWeeksAgo := time.Now().AddDate(0, 0, -14)
	oneWeekAgo := time.Now().AddDate(0, 0, -7)
	twoDaysAgo := time.Now().AddDate(0, 0, -2)

	// Create workout history - 3 previous workouts
	return []workout.Session{
		{
			// 2 weeks ago - Monday workout
			WorkoutDate:      twoWeeksAgo,
			Status:           workout.StatusDone,
			DifficultyRating: ptr.Ref(3), // Medium difficulty
			StartedAt:        ptr.Ref(twoWeeksAgo.Add(17 * time.Hour)),
			CompletedAt:      ptr.Ref(twoWeeksAgo.Add(18 * time.Hour)),
			ExerciseSets: []workout.ExerciseSet{
				{
					Exercise: exercises[0], // Bench Press
					Sets: []workout.Set{
						{WeightKg: 80, MinReps: 8, MaxReps: 12, CompletedReps: &rep10},
						{WeightKg: 80, MinReps: 8, MaxReps: 12, CompletedReps: &rep8},
						{WeightKg: 80, MinReps: 8, MaxReps: 12, CompletedReps: &rep8},
					},
				},
				{
					Exercise: exercises[2], // Shoulder Press
					Sets: []workout.Set{
						{WeightKg: 50, MinReps: 8, MaxReps: 12, CompletedReps: &rep12},
						{WeightKg: 50, MinReps: 8, MaxReps: 12, CompletedReps: &rep10},
						{WeightKg: 50, MinReps: 8, MaxReps: 12, CompletedReps: &rep8},
					},
				},
			},
		},
		{
			// 1 week ago - Monday workout
			WorkoutDate:      oneWeekAgo,
			Status:           workout.StatusDone,
			DifficultyRating: ptr.Ref(4), // Somewhat challenging
			StartedAt:        ptr.Ref(oneWeekAgo.Add(17 * time.Hour)),
			CompletedAt:      ptr.Ref(oneWeekAgo.Add(18 * time.Hour)),
			ExerciseSets: []workout.ExerciseSet{
				{
					Exercise: exercises[0], // Bench Press
					Sets: []workout.Set{
						{WeightKg: 82., MinReps: 8, MaxReps: 12, CompletedReps: &rep10},
						{WeightKg: 82., MinReps: 8, MaxReps: 12, CompletedReps: &rep8},
						{WeightKg: 82., MinReps: 8, MaxReps: 12, CompletedReps: &rep6},
					},
				},
				{
					Exercise: exercises[2], // Shoulder Press
					Sets: []workout.Set{
						{WeightKg: 52.5, MinReps: 8, MaxReps: 12, CompletedReps: &rep10},
						{WeightKg: 52.5, MinReps: 8, MaxReps: 12, CompletedReps: &rep8},
						{WeightKg: 52.5, MinReps: 8, MaxReps: 12, CompletedReps: &rep8},
					},
				},
			},
		},
		{
			// 2 days ago - Saturday workout
			WorkoutDate:      twoDaysAgo,
			Status:           workout.StatusDone,
			DifficultyRating: ptr.Ref(2), // Somewhat easy
			StartedAt:        ptr.Ref(twoDaysAgo.Add(10 * time.Hour)),
			CompletedAt:      ptr.Ref(twoDaysAgo.Add(11 * time.Hour)),
			ExerciseSets: []workout.ExerciseSet{
				{
					Exercise: exercises[6], // Squat
					Sets: []workout.Set{
						{WeightKg: 100, MinReps: 8, MaxReps: 12, CompletedReps: &rep12},
						{WeightKg: 100, MinReps: 8, MaxReps: 12, CompletedReps: &rep12},
						{WeightKg: 100, MinReps: 8, MaxReps: 12, CompletedReps: &rep10},
					},
				},
				{
					Exercise: exercises[7], // Deadlift
					Sets: []workout.Set{
						{WeightKg: 120, MinReps: 3, MaxReps: 6, CompletedReps: &rep6},
						{WeightKg: 120, MinReps: 3, MaxReps: 6, CompletedReps: &rep6},
						{WeightKg: 120, MinReps: 3, MaxReps: 6, CompletedReps: &rep6},
					},
				},
			},
		},
	}
}

// TestProgressionOverTime verifies that the workout generator
// correctly progresses workouts over time.
func TestProgressionOverTime(t *testing.T) {
	// Create initial generator with test data.
	preferences := workout.Preferences{
		Monday:    true,
		Wednesday: true,
		Friday:    true,
		Tuesday:   false,
		Thursday:  false,
		Saturday:  false,
		Sunday:    false,
	}
	exercises := createExercisePool()
	initialHistory := []workout.Session{}

	// Create initial generator
	gen := generator.NewGenerator(preferences, initialHistory, exercises)

	// Generate a series of workouts over time and complete them
	// to simulate progressive overload
	history := initialHistory
	simulationWeeks := 4
	startDate := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC) // Start on a Monday

	// For each simulated week
	for weekIndex := range simulationWeeks {
		// For each workout day in the week
		for _, dayOffset := range []int{0, 2, 4} { // Monday, Wednesday, Friday
			workoutDate := startDate.AddDate(0, 0, weekIndex*7+dayOffset)

			// Generate workout
			session, err := gen.Generate(workoutDate)
			if err != nil {
				t.Fatalf("Failed to generate workout for week %d, day %d: %v", weekIndex+1, dayOffset, err)
			}

			// Complete the workout (simulate user completing it)
			completedSession := simulateWorkoutCompletion(session, weekIndex)

			// Add to history
			history = append(history, completedSession)

			// Create a new generator with updated history for next workout
			gen = generator.NewGenerator(preferences, history, exercises)
		}
	}

	// Verify progression over time
	verifyWorkoutProgression(t, history)
}

// TestContinuityBetweenWeeks verifies that the workout generator maintains
// appropriate exercise continuity between weeks.
func TestContinuityBetweenWeeks(t *testing.T) {
	// Create test data
	preferences := workout.Preferences{
		Monday:    true,
		Friday:    true,
		Tuesday:   false,
		Wednesday: false,
		Thursday:  false,
		Saturday:  false,
		Sunday:    false,
	}
	exercises := createExercisePool()

	// Generate several workouts across different weeks on the same weekday
	history := []workout.Session{}
	numWeeks := 3
	gen := generator.NewGenerator(preferences, history, exercises)

	mondaySessions := make([]workout.Session, numWeeks)

	// Generate Monday workouts for several weeks
	for weekIndex := range numWeeks {
		date := time.Date(2023, 1, 2+weekIndex*7, 0, 0, 0, 0, time.UTC) // Mondays
		session, err := gen.Generate(date)
		if err != nil {
			t.Fatalf("Failed to generate workout for week %d: %v", weekIndex+1, err)
		}

		// Complete the workout and add to history
		completedSession := simulateWorkoutCompletion(session, weekIndex)
		history = append(history, completedSession)
		mondaySessions[weekIndex] = completedSession

		// Update generator with new history
		gen = generator.NewGenerator(preferences, history, exercises)
	}

	// Verify continuity between weeks
	verifyContinuityBetweenWeeks(t, mondaySessions)
}

// TestUserFeedbackIntegration tests how the workout generator responds to user feedback.
func TestUserFeedbackIntegration(t *testing.T) {
	// Setup test data
	preferences := workout.Preferences{
		Tuesday:   true,
		Thursday:  true,
		Monday:    false,
		Wednesday: false,
		Friday:    false,
		Saturday:  false,
		Sunday:    false,
	}
	exercises := createExercisePool()

	// Create initial workout and add to history
	initialDate := time.Date(2023, 1, 3, 0, 0, 0, 0, time.UTC) // Tuesday
	gen := generator.NewGenerator(preferences, nil, exercises)

	initialSession, err := gen.Generate(initialDate)
	if err != nil {
		t.Fatalf("Failed to generate initial workout: %v", err)
	}

	// First complete the initial workout with a moderate rating
	// This establishes starting weights for exercises
	initialCompletedSession := simulateWorkoutCompletion(initialSession, 0)
	initialHistory := []workout.Session{initialCompletedSession}

	// For each feedback level, create a new test session
	testSessions := []struct {
		rating  int
		session workout.Session
	}{}

	// For each feedback level
	for _, rating := range []int{1, 3, 5} { // Too easy, Optimal, Too difficult
		// Generate workout with initial history
		gen = generator.NewGenerator(preferences, initialHistory, exercises)

		testDate := initialDate.AddDate(0, 0, 2) // Thursday
		session, genErr := gen.Generate(testDate)
		if genErr != nil {
			t.Fatalf("Failed to generate test workout: %v", genErr)
		}

		// Complete the workout with specific feedback
		completedSession := simulateWorkoutCompletionWithFeedback(session, 0, rating)

		testSessions = append(testSessions, struct {
			rating  int
			session workout.Session
		}{rating, completedSession})
	}

	// For each type of feedback, generate a new workout
	for _, testCase := range testSessions {
		rating := testCase.rating
		completedSession := testCase.session

		// Create a new history with initial session and current completed session
		var updatedHistory = append(initialHistory, completedSession)
		updatedGen := generator.NewGenerator(preferences, updatedHistory, exercises)

		// Generate next workout
		nextDate := initialDate.AddDate(0, 0, 7) // Next Tuesday
		nextSession, nextErr := updatedGen.Generate(nextDate)
		if nextErr != nil {
			t.Fatalf("Failed to generate workout after feedback %d: %v", rating, nextErr)
		}

		// Verify appropriate response to feedback
		verifyFeedbackResponse(t, completedSession, nextSession)
	}
}

// Helper functions for test cases

// Simulate a user completing a workout with a specified difficulty rating.
func simulateWorkoutCompletionWithFeedback(session workout.Session, week int, difficultyRating int) workout.Session {
	// Start with basic completion
	completed := simulateWorkoutCompletion(session, week)

	// Add difficulty rating
	completed.DifficultyRating = &difficultyRating

	return completed
}

// Simulate a user completing a workout.
func simulateWorkoutCompletion(session workout.Session, week int) workout.Session {
	// Copy the session
	completed := session

	// Set status to done
	completed.Status = workout.StatusDone

	// Set start and end times
	startTime := session.WorkoutDate.Add(17 * time.Hour) // 5 PM
	endTime := startTime.Add(1 * time.Hour)              // 1 hour workout
	completed.StartedAt = &startTime
	completed.CompletedAt = &endTime

	// Simulate completing each set
	for i := range completed.ExerciseSets {
		for j := range completed.ExerciseSets[i].Sets {
			// Simulate the user completing the sets
			set := &completed.ExerciseSets[i].Sets[j]

			// If weight is 0, assign a reasonable starting weight based on exercise
			if set.WeightKg == 0 {
				// Set a starting weight based on exercise type and primary muscle groups
				exerciseName := completed.ExerciseSets[i].Exercise.Name
				set.WeightKg = determineStartingWeight(exerciseName)
			}

			// Determine completion level (improve over weeks)
			completionLevel := float64(week) * 0.1 // 0%, 10%, 20%, etc. improvement per week

			// Calculate completed reps between min and max
			repRange := set.MaxReps - set.MinReps
			improvement := int(float64(repRange) * completionLevel)
			completedReps := set.MinReps + improvement

			// Ensure we don't exceed max reps
			if completedReps > set.MaxReps {
				completedReps = set.MaxReps
			}

			set.CompletedReps = &completedReps
		}
	}

	// Random difficulty rating if not set elsewhere (between 2-4)
	if completed.DifficultyRating == nil {
		rating := 3 // Default to moderate
		completed.DifficultyRating = &rating
	}

	return completed
}

// Helper function to determine a reasonable starting weight based on exercise.
func determineStartingWeight(exerciseName string) float64 {
	// Map common exercises to reasonable starting weights
	// These are just examples and should be adjusted for your app's context
	switch exerciseName {
	case "Bench Press":
		return 60.0
	case "Pull-Up", "Barbell Row":
		return 50.0
	case "Overhead Press", "Push Press":
		return 40.0
	case "Squat":
		return 70.0
	case "Deadlift":
		return 80.0
	case "Romanian Deadlift":
		return 60.0
	case "Lunges":
		return 40.0
	case "Power Clean":
		return 50.0
	case "Kettlebell Swing":
		return 20.0
	case "Thruster":
		return 40.0
	default:
		// For unknown exercises, provide a moderate default
		return 30.0
	}
}

// Verify that workouts progress over time (weights increase).
func verifyWorkoutProgression(t *testing.T, history []workout.Session) {
	t.Helper()

	// Map to store weight progressions per exercise
	exerciseProgression := make(map[int][]float64)

	// Extract weight data from history
	for _, session := range history {
		for _, exerciseSet := range session.ExerciseSets {
			exerciseID := exerciseSet.Exercise.ID

			// Use the weight of the first set as representative
			if len(exerciseSet.Sets) > 0 {
				exerciseProgression[exerciseID] = append(
					exerciseProgression[exerciseID],
					exerciseSet.Sets[0].WeightKg,
				)
			}
		}
	}

	// Check for progression in exercises that appear multiple times
	progressionFound := false
	nonZeroWeightsFound := false

	for _, weights := range exerciseProgression {
		if len(weights) < 2 {
			continue // Skip exercises that only appear once
		}

		// First check if weights are non-zero, which indicates the user has set weights
		for _, weight := range weights {
			if weight > 0 {
				nonZeroWeightsFound = true
				break
			}
		}

		// Check if weights generally increase over time (allowing for deloads)
		increasesCount := 0
		decreasesCount := 0

		for i := 1; i < len(weights); i++ {
			if weights[i] > weights[i-1] {
				increasesCount++
			} else if weights[i] < weights[i-1] {
				decreasesCount++
			}
		}

		// We expect more increases than decreases for progression
		if increasesCount > decreasesCount {
			progressionFound = true
		}
	}

	// If we didn't find progression but did find non-zero weights, that's a problem
	if !progressionFound && nonZeroWeightsFound {
		t.Error("No exercise showed weight progression over time despite having non-zero weights")
	}
}

// Verify appropriate continuity between workouts on the same day of the week.
func verifyContinuityBetweenWeeks(t *testing.T, sessions []workout.Session) {
	t.Helper()

	if len(sessions) < 2 {
		t.Skip("Need at least 2 sessions to verify continuity")
	}

	// For each pair of consecutive workouts
	for i := 1; i < len(sessions); i++ {
		currentExercises := getExerciseIDs(sessions[i])
		previousExercises := getExerciseIDs(sessions[i-1])

		// Calculate continuity percentage
		commonExercises := countCommonElements(currentExercises, previousExercises)
		continuityPercentage := float64(commonExercises) / float64(len(previousExercises)) * 100

		// We expect around 80% continuity
		if continuityPercentage < 60 {
			t.Errorf("Continuity between weeks %d and %d is too low: %.1f%% (expected ~80%%)",
				i, i+1, continuityPercentage)
		} else {
			t.Logf("Continuity between weeks %d and %d: %.1f%%", i, i+1, continuityPercentage)
		}
	}
}

// Verify appropriate response to user feedback.
func verifyFeedbackResponse(t *testing.T, previousSession, nextSession workout.Session) {
	t.Helper()

	if previousSession.DifficultyRating == nil {
		t.Skip("Previous session has no difficulty rating")
	}

	rating := *previousSession.DifficultyRating

	// Find exercises that exist in both workouts for comparison
	commonExercises := findCommonExercises(previousSession, nextSession)

	if len(commonExercises) == 0 {
		t.Errorf("No common exercises found between workouts to verify feedback response")
	}

	// Check weight changes based on feedback
	for _, pair := range commonExercises {
		prevWeight := getAverageWeight(pair.previous)
		nextWeight := getAverageWeight(pair.next)

		// Log exercise details for debugging
		t.Logf("Exercise %s (ID: %d): Previous weight: %.1f, Next weight: %.1f",
			pair.previous.Exercise.Name, pair.previous.Exercise.ID, prevWeight, nextWeight)

		switch rating {
		case 1: // Too easy
			// Should see weight increase after 'too easy' feedback
			// But only if there was weight to begin with
			if prevWeight > 0 && nextWeight <= prevWeight {
				// If weight dropped to 0, it indicates a likely matching issue
				if nextWeight == 0 {
					t.Errorf("Weight reset to 0 for %s (ID: %d) after 'too easy' feedback - likely an exercise matching issue",
						pair.previous.Exercise.Name, pair.previous.Exercise.ID)
				} else {
					t.Errorf("Expected weight increase after 'too easy' feedback for %s (ID: %d), but got %.1f -> %.1f",
						pair.previous.Exercise.Name, pair.previous.Exercise.ID, prevWeight, nextWeight)
				}
			} else if prevWeight == 0 && nextWeight == 0 {
				// If weights are still at 0, that's okay for the first workout since user needs to select weight
				t.Logf("Weight remains at 0 for %s after 'too easy' feedback - this is expected for initial workouts",
					pair.previous.Exercise.Name)
			}
		case 5: // Too difficult
			// Should see weight decrease or volume reduction
			// But if weight is already 0, we can't decrease further
			prevSets := len(pair.previous.Sets)
			nextSets := len(pair.next.Sets)

			if prevWeight > 0 && nextWeight >= prevWeight && nextSets >= prevSets {
				t.Errorf("Expected weight decrease or volume reduction after 'too difficult' feedback for %s (ID: %d), "+
					"but got %.1f -> %.1f, sets: %d -> %d",
					pair.previous.Exercise.Name, pair.previous.Exercise.ID, prevWeight, nextWeight, prevSets, nextSets)
			} else if prevWeight == 0 && nextSets >= prevSets {
				// If weight is already 0, we should at least reduce volume
				t.Logf("Weight already at 0 for %s after 'too difficult' feedback - expected volume reduction",
					pair.previous.Exercise.Name)
			}
		default: // Optimal (2-4)
			// More forgiving check for small weight changes (within 15%)
			if prevWeight > 0 && nextWeight > 0 &&
				(nextWeight < prevWeight*0.85 || nextWeight > prevWeight*1.15) {
				// For undulating periodization, allow larger weight changes but flag potential issues
				if nextWeight == 0 {
					t.Errorf("Weight reset to 0 for %s (ID: %d) after 'optimal' feedback - likely an exercise matching issue",
						pair.previous.Exercise.Name, pair.previous.Exercise.ID)
				} else {
					t.Errorf("Expected moderate weight changes after 'optimal' feedback for %s (ID: %d), but got %.1f -> %.1f",
						pair.previous.Exercise.Name, pair.previous.Exercise.ID, prevWeight, nextWeight)
				}
			} else if prevWeight > 0 && nextWeight == 0 {
				t.Errorf("Weight reset to 0 for %s (ID: %d) after 'optimal' feedback - likely an exercise matching issue",
					pair.previous.Exercise.Name, pair.previous.Exercise.ID)
			}
		}
	}
}

// Helper functions for data manipulation

// Get list of exercise IDs from a session.
func getExerciseIDs(session workout.Session) []int {
	ids := make([]int, 0, len(session.ExerciseSets))
	for _, exerciseSet := range session.ExerciseSets {
		ids = append(ids, exerciseSet.Exercise.ID)
	}
	return ids
}

// Count common elements between two slices.
func countCommonElements(slice1, slice2 []int) int {
	set := make(map[int]bool)
	for _, val := range slice1 {
		set[val] = true
	}

	count := 0
	for _, val := range slice2 {
		if set[val] {
			count++
		}
	}

	return count
}

// Exercise pair for comparison.
type exercisePair struct {
	previous workout.ExerciseSet
	next     workout.ExerciseSet
}

// FindCommonExercises finds exercises that exist in both workouts.
func findCommonExercises(prev, next workout.Session) []exercisePair {
	var common []exercisePair

	for _, prevEx := range prev.ExerciseSets {
		for _, nextEx := range next.ExerciseSets {
			// Match by both ID and name for more reliable matching
			if prevEx.Exercise.ID == nextEx.Exercise.ID ||
				prevEx.Exercise.Name == nextEx.Exercise.Name {
				common = append(common, exercisePair{
					previous: prevEx,
					next:     nextEx,
				})
				break
			}
		}
	}

	return common
}

// Get average weight across all sets for an exercise.
func getAverageWeight(exerciseSet workout.ExerciseSet) float64 {
	if len(exerciseSet.Sets) == 0 {
		return 0
	}

	var total float64
	for _, set := range exerciseSet.Sets {
		total += set.WeightKg
	}

	return total / float64(len(exerciseSet.Sets))
}
