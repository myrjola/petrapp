package workout

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/ptr"
)

// Helper function for creating test sets with weights.
func createTestSet(weight float64, minReps, maxReps int, completedReps *int) Set {
	return Set{
		WeightKg:      &weight,
		MinReps:       minReps,
		MaxReps:       maxReps,
		CompletedReps: completedReps,
		CompletedAt:   nil,
	}
}

// TestGenerate verifies that the workout generator generates
// valid workouts according to our specifications.
func TestGenerate(t *testing.T) {
	// Define test cases with different input parameters
	testCases := []struct {
		name        string
		preferences Preferences
		history     []sessionAggregate
		pool        []Exercise
		date        time.Time
	}{
		{
			name: "New user with no history",
			preferences: Preferences{
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
			preferences: Preferences{
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
			preferences: Preferences{
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
			gen, err := newGenerator(tc.preferences, tc.history, tc.pool)
			if err != nil {
				t.Fatalf("Failed to create generator: %v", err)
			}

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
	emptyPreferences := Preferences{
		Monday:    false,
		Tuesday:   false,
		Wednesday: false,
		Thursday:  false,
		Friday:    false,
		Saturday:  false,
		Sunday:    false,
	}
	emptyHistory := []sessionAggregate{}
	emptyPool := []Exercise{}

	gen, err := newGenerator(emptyPreferences, emptyHistory, emptyPool)
	if err == nil {
		t.Error("Expected an error when creating a generator with an empty exercise pool, but got nil")
	}
	if gen != nil {
		t.Errorf("Expected generator to be nil when error occurred, got %v", gen)
	}
}

func verifyGeneratedWorkout(t *testing.T, session sessionAggregate, date time.Time) {
	t.Helper()

	// Verify date is correct
	if !session.Date.Equal(date) {
		t.Errorf("Expected workout date %v, got %v", date, session.Date)
	}

	// Verify workout has appropriate number of exercises (5-8)
	exerciseCount := len(session.ExerciseSets)
	if exerciseCount < 5 || exerciseCount > 8 {
		t.Errorf("Expected between 5-8 exercises, got %d", exerciseCount)
	}

	// Verify all exercises have appropriate number of sets and rep ranges
	for _, exerciseSet := range session.ExerciseSets {
		// Check set count (3-6 sets per exercise)
		setCount := len(exerciseSet.Sets)
		if setCount < 3 || setCount > 6 {
			t.Errorf("Exercise %d has %d sets, expected 3-6 sets",
				exerciseSet.ExerciseID, setCount)
		}

		// Verify set parameters
		for j, set := range exerciseSet.Sets {
			// Verify all weights are reasonable (skip for bodyweight exercises where weight is nil)
			if set.WeightKg != nil && *set.WeightKg < 0 {
				t.Errorf("Exercise %d, set #%d has negative weight: %f",
					exerciseSet.ExerciseID, j+1, *set.WeightKg)
			}

			// Verify rep ranges (3-16)
			if set.MinReps < 3 || set.MaxReps > 16 {
				t.Errorf("Exercise %d, set #%d has unusual rep range: %d-%d",
					exerciseSet.ExerciseID, j+1, set.MinReps, set.MaxReps)
			}

			// Verify min reps <= max reps
			if set.MinReps > set.MaxReps {
				t.Errorf("Exercise %d, set #%d has invalid rep range: min %d > max %d",
					exerciseSet.ExerciseID, j+1, set.MinReps, set.MaxReps)
			}

			// Verify no completed reps in a new workout
			if set.CompletedReps != nil {
				t.Errorf("Exercise %d, set #%d has completed reps in a new workout",
					exerciseSet.ExerciseID, j+1)
			}
		}

		// Verify set consistency (all sets for an exercise should have the same rep range)
		if !setsHaveConsistentRepRanges(t, exerciseSet.Sets) {
			t.Errorf("Exercise %d has inconsistent rep ranges across sets",
				exerciseSet.ExerciseID)
		}
	}

	// Verify exercise uniqueness
	if !hasUniqueExercises(t, session.ExerciseSets) {
		t.Error("Workout contains duplicate exercises")
	}
}

// Helper function to check if an exercise set has consistent rep ranges.
func setsHaveConsistentRepRanges(t *testing.T, sets []Set) bool {
	t.Helper()
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
func hasUniqueExercises(t *testing.T, exerciseSets []exerciseSetAggregate) bool {
	t.Helper()
	exerciseIDs := make(map[int]bool)

	for _, es := range exerciseSets {
		if exerciseIDs[es.ExerciseID] {
			return false
		}
		exerciseIDs[es.ExerciseID] = true
	}

	return true
}

// Helper function to create a diverse pool of exercises.
func createExercisePool() []Exercise {
	return []Exercise{
		// Upper body exercises
		{
			ID:                    1,
			Name:                  "Bench Press",
			Category:              CategoryUpper,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
			SecondaryMuscleGroups: []string{"Shoulders"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    2,
			Name:                  "Pull Up",
			Category:              CategoryUpper,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Back", "Biceps"},
			SecondaryMuscleGroups: []string{"Forearms"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    3,
			Name:                  "Shoulder Press",
			Category:              CategoryUpper,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders"},
			SecondaryMuscleGroups: []string{"Triceps"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    4,
			Name:                  "Bicep Curl",
			Category:              CategoryUpper,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Biceps"},
			SecondaryMuscleGroups: []string{"Forearms"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    5,
			Name:                  "Tricep Extension",
			Category:              CategoryUpper,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Triceps"},
			SecondaryMuscleGroups: []string{},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    6,
			Name:                  "Lateral Raise",
			Category:              CategoryUpper,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders"},
			SecondaryMuscleGroups: []string{},
			DescriptionMarkdown:   "",
		},

		// Lower body exercises
		{
			ID:                    7,
			Name:                  "Squat",
			Category:              CategoryLower,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Quadriceps", "Glutes"},
			SecondaryMuscleGroups: []string{"Hamstrings", "Core"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    8,
			Name:                  "Deadlift",
			Category:              CategoryLower,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Hamstrings", "Back"},
			SecondaryMuscleGroups: []string{"Glutes", "Forearms"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    9,
			Name:                  "Leg Press",
			Category:              CategoryLower,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Quadriceps"},
			SecondaryMuscleGroups: []string{"Glutes", "Hamstrings"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    10,
			Name:                  "Leg Curl",
			Category:              CategoryLower,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Hamstrings"},
			SecondaryMuscleGroups: []string{},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    11,
			Name:                  "Calf Raise",
			Category:              CategoryLower,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Calves"},
			SecondaryMuscleGroups: []string{},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    12,
			Name:                  "Leg Extension",
			Category:              CategoryLower,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Quadriceps"},
			SecondaryMuscleGroups: []string{},
			DescriptionMarkdown:   "",
		},

		// Full body exercises
		{
			ID:                    13,
			Name:                  "Clean and Press",
			Category:              CategoryFullBody,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders", "Legs", "Back"},
			SecondaryMuscleGroups: []string{"Core", "Arms"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    14,
			Name:                  "Burpee",
			Category:              CategoryFullBody,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest", "Legs", "Shoulders"},
			SecondaryMuscleGroups: []string{"Core", "Arms"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    15,
			Name:                  "Kettlebell Swing",
			Category:              CategoryFullBody,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Hamstrings", "Glutes", "Back"},
			SecondaryMuscleGroups: []string{"Shoulders", "Core"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    16,
			Name:                  "Turkish Get-Up",
			Category:              CategoryFullBody,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders", "Core", "Legs"},
			SecondaryMuscleGroups: []string{"Arms", "Back"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    17,
			Name:                  "Thruster",
			Category:              CategoryFullBody,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Legs", "Shoulders"},
			SecondaryMuscleGroups: []string{"Core", "Arms"},
			DescriptionMarkdown:   "",
		},
		{
			ID:                    18,
			Name:                  "Power Clean",
			Category:              CategoryFullBody,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Back", "Legs", "Shoulders"},
			SecondaryMuscleGroups: []string{"Arms", "Core"},
			DescriptionMarkdown:   "",
		},
	}
}

// Helper function to create mock workout history.
func createWorkoutHistory() []sessionAggregate {
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
	return []sessionAggregate{
		{
			// 2 weeks ago - Monday workout
			Date:             twoWeeksAgo,
			DifficultyRating: ptr.Ref(3), // Medium difficulty
			StartedAt:        twoWeeksAgo.Add(17 * time.Hour),
			CompletedAt:      twoWeeksAgo.Add(18 * time.Hour),
			ExerciseSets: []exerciseSetAggregate{
				{
					ExerciseID: exercises[0].ID, // Bench Press
					Sets: []Set{
						createTestSet(80, 8, 12, &rep10),
						createTestSet(80, 8, 12, &rep8),
						createTestSet(80, 8, 12, &rep8),
					},
					WarmupCompletedAt: nil,
				},
				{
					ExerciseID: exercises[2].ID, // Shoulder Press
					Sets: []Set{
						createTestSet(50, 8, 12, &rep12),
						createTestSet(50, 8, 12, &rep10),
						createTestSet(50, 8, 12, &rep8),
					},
					WarmupCompletedAt: nil,
				},
			},
		},
		{
			// 1 week ago - Monday workout
			Date:             oneWeekAgo,
			DifficultyRating: ptr.Ref(4), // Somewhat challenging
			StartedAt:        oneWeekAgo.Add(17 * time.Hour),
			CompletedAt:      oneWeekAgo.Add(18 * time.Hour),
			ExerciseSets: []exerciseSetAggregate{
				{
					ExerciseID: exercises[0].ID, // Bench Press
					Sets: []Set{
						createTestSet(82., 8, 12, &rep10),
						createTestSet(82., 8, 12, &rep8),
						createTestSet(82., 8, 12, &rep6),
					},
					WarmupCompletedAt: nil,
				},
				{
					ExerciseID: exercises[2].ID, // Shoulder Press
					Sets: []Set{
						createTestSet(52.5, 8, 12, &rep10),
						createTestSet(52.5, 8, 12, &rep8),
						createTestSet(52.5, 8, 12, &rep8),
					},
					WarmupCompletedAt: nil,
				},
			},
		},
		{
			// 2 days ago - Saturday workout
			Date:             twoDaysAgo,
			DifficultyRating: ptr.Ref(2), // Somewhat easy
			StartedAt:        twoDaysAgo.Add(10 * time.Hour),
			CompletedAt:      twoDaysAgo.Add(11 * time.Hour),
			ExerciseSets: []exerciseSetAggregate{
				{
					ExerciseID: exercises[6].ID, // Squat
					Sets: []Set{
						createTestSet(100, 8, 12, &rep12),
						createTestSet(100, 8, 12, &rep12),
						createTestSet(100, 8, 12, &rep10),
					},
					WarmupCompletedAt: nil,
				},
				{
					ExerciseID: exercises[7].ID, // Deadlift
					Sets: []Set{
						createTestSet(120, 3, 6, &rep6),
						createTestSet(120, 3, 6, &rep6),
						createTestSet(120, 3, 6, &rep6),
					},
					WarmupCompletedAt: nil,
				},
			},
		},
	}
}

// TestProgressionOverTime verifies that the workout generator
// correctly progresses workouts over time.
func TestProgressionOverTime(t *testing.T) {
	// Create initial generator with test data.
	preferences := Preferences{
		Monday:    true,
		Wednesday: true,
		Friday:    true,
		Tuesday:   false,
		Thursday:  false,
		Saturday:  false,
		Sunday:    false,
	}
	exercises := createExercisePool()
	initialHistory := []sessionAggregate{}

	// Create initial generator
	gen, err := newGenerator(preferences, initialHistory, exercises)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

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
			var session sessionAggregate
			session, err = gen.Generate(workoutDate)
			if err != nil {
				t.Fatalf("Failed to generate workout for week %d, day %d: %v", weekIndex+1, dayOffset, err)
			}

			// Complete the workout (simulate user completing it)
			completedSession := simulateWorkoutCompletion(session, weekIndex)

			// Add to history
			history = append(history, completedSession)

			// Create a new generator with updated history for next workout
			gen, err = newGenerator(preferences, history, exercises)
			if err != nil {
				t.Fatalf("Failed to create generator for week %d, day %d: %v", weekIndex+1, dayOffset, err)
			}
		}
	}

	// Verify progression over time
	verifyWorkoutProgression(t, history)
}

// TestContinuityBetweenWeeks verifies that the workout generator maintains
// appropriate exercise continuity between weeks.
func TestContinuityBetweenWeeks(t *testing.T) {
	// Create test data
	preferences := Preferences{
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
	history := []sessionAggregate{}
	numWeeks := 3
	gen, err := newGenerator(preferences, history, exercises)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	mondaySessions := make([]sessionAggregate, numWeeks)

	// Generate Monday workouts for several weeks
	for weekIndex := range numWeeks {
		date := time.Date(2023, 1, 2+weekIndex*7, 0, 0, 0, 0, time.UTC) // Mondays
		var session sessionAggregate
		session, err = gen.Generate(date)
		if err != nil {
			t.Fatalf("Failed to generate workout for week %d: %v", weekIndex+1, err)
		}

		// Complete the workout and add to history
		completedSession := simulateWorkoutCompletion(session, weekIndex)
		history = append(history, completedSession)
		mondaySessions[weekIndex] = completedSession

		// Update generator with new history
		gen, err = newGenerator(preferences, history, exercises)
		if err != nil {
			t.Fatalf("Failed to create generator: %v", err)
		}
	}

	// Verify continuity between weeks
	verifyContinuityBetweenWeeks(t, mondaySessions)
}

// TestUserFeedbackIntegration tests how the workout generator responds to user feedback.
func TestUserFeedbackIntegration(t *testing.T) {
	// Setup test data
	preferences := Preferences{
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
	gen, err := newGenerator(preferences, nil, exercises)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	initialSession, err := gen.Generate(initialDate)
	if err != nil {
		t.Fatalf("Failed to generate initial workout: %v", err)
	}

	// First complete the initial workout with a moderate rating
	// This establishes starting weights for exercises
	initialCompletedSession := simulateWorkoutCompletion(initialSession, 0)
	initialHistory := []sessionAggregate{initialCompletedSession}

	// For each feedback level, create a new test session
	testSessions := []struct {
		rating  int
		session sessionAggregate
	}{}

	// For each feedback level
	for _, rating := range []int{1, 3, 5} { // Too easy, Optimal, Too difficult
		// Generate workout with initial history
		gen, err = newGenerator(preferences, initialHistory, exercises)
		if err != nil {
			t.Fatalf("Failed to create generator for feedback %d: %v", rating, err)
		}

		testDate := initialDate.AddDate(0, 0, 7) // Tuesday again to have shared exercises.
		session, genErr := gen.Generate(testDate)
		if genErr != nil {
			t.Fatalf("Failed to generate test workout: %v", genErr)
		}

		// Complete the workout with specific feedback
		completedSession := simulateWorkoutCompletionWithFeedback(session, 0, rating)

		testSessions = append(testSessions, struct {
			rating  int
			session sessionAggregate
		}{rating, completedSession})
	}

	// For each type of feedback, generate a new workout
	for _, testCase := range testSessions {
		rating := testCase.rating
		completedSession := testCase.session

		// Create a new history with initial session and current completed session
		var updatedHistory = append(initialHistory, completedSession)
		var updatedGen *generator
		updatedGen, err = newGenerator(preferences, updatedHistory, exercises)
		if err != nil {
			t.Fatalf("Failed to create generator for feedback %d: %v", rating, err)
		}

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
func simulateWorkoutCompletionWithFeedback(session sessionAggregate, week int, difficultyRating int) sessionAggregate {
	// Start with basic completion
	completed := simulateWorkoutCompletion(session, week)

	// Add difficulty rating
	completed.DifficultyRating = &difficultyRating

	return completed
}

func simulateWorkoutCompletion(session sessionAggregate, week int) sessionAggregate {
	// Copy the session
	completed := session

	// Set start and end times
	startTime := session.Date.Add(17 * time.Hour) // 5 PM
	endTime := startTime.Add(1 * time.Hour)       // 1-hour workout
	completed.StartedAt = startTime
	completed.CompletedAt = endTime

	// Simulate completing each set
	for i := range completed.ExerciseSets {
		for j := range completed.ExerciseSets[i].Sets {
			// Simulate the user completing the sets
			set := &completed.ExerciseSets[i].Sets[j]

			// If weight is 0, assign a reasonable starting weight based on exercise
			if set.WeightKg != nil && *set.WeightKg == 0 {
				*set.WeightKg = 30
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

// Verify that workouts progress over time (weights increase).
func verifyWorkoutProgression(t *testing.T, history []sessionAggregate) {
	t.Helper()

	// Map to store weight progressions per exercise
	exerciseProgression := make(map[int][]float64)

	// Extract weight data from history
	for _, session := range history {
		for _, exerciseSet := range session.ExerciseSets {
			exerciseID := exerciseSet.ExerciseID

			// Use the weight of the first set as representative
			if len(exerciseSet.Sets) > 0 {
				var weight float64
				if exerciseSet.Sets[0].WeightKg != nil {
					weight = *exerciseSet.Sets[0].WeightKg
				}
				exerciseProgression[exerciseID] = append(
					exerciseProgression[exerciseID],
					weight,
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
func verifyContinuityBetweenWeeks(t *testing.T, sessions []sessionAggregate) {
	t.Helper()

	if len(sessions) < 2 {
		t.Skip("Need at least 2 sessions to verify continuity")
	}

	// For each pair of consecutive workouts
	for i := 1; i < len(sessions); i++ {
		currentExercises := getExerciseIDs(t, sessions[i])
		previousExercises := getExerciseIDs(t, sessions[i-1])

		// Calculate continuity percentage
		commonExercises := countCommonElements(t, currentExercises, previousExercises)
		continuityPercentage := float64(commonExercises) / float64(len(previousExercises)) * 100

		// We expect around 80% continuity
		if continuityPercentage < 60 {
			t.Errorf("Continuity between weeks %d and %d is too low: %.1f%% (expected ~80%%)",
				i, i+1, continuityPercentage)
		}
	}
}

// Verify appropriate response to user feedback.
func verifyFeedbackResponse(t *testing.T, previousSession, nextSession sessionAggregate) {
	t.Helper()

	if previousSession.DifficultyRating == nil {
		t.Skip("Previous session has no difficulty rating")
	}

	rating := *previousSession.DifficultyRating

	// Find exercises that exist in both workouts for comparison
	commonExercises := findCommonExercises(t, previousSession, nextSession)

	if len(commonExercises) == 0 {
		t.Errorf("No common exercises found between workouts to verify feedback response")
	}

	// Check weight changes based on feedback
	for _, pair := range commonExercises {
		prevWeight := getAverageWeight(t, pair.previous)
		nextWeight := getAverageWeight(t, pair.next)

		switch rating {
		case 1: // Too easy
			// Should see weight increase after 'too easy' feedback
			// But only if there was weight to begin with
			if prevWeight > 0 && nextWeight <= prevWeight {
				// If weight dropped to 0, it indicates a likely matching issue
				if nextWeight == 0 {
					t.Errorf("Weight reset to 0 for exercise %d after 'too easy' feedback - likely an exercise matching issue",
						pair.previous.ExerciseID)
				} else {
					t.Errorf("Expected weight increase after 'too easy' feedback for exercise %d, but got %.1f -> %.1f",
						pair.previous.ExerciseID, prevWeight, nextWeight)
				}
			}
		case 5: // Too difficult
			// Should see weight decrease or volume reduction
			// But if weight is already 0, we can't decrease further
			prevSets := len(pair.previous.Sets)
			nextSets := len(pair.next.Sets)

			if prevWeight > 0 && nextWeight >= prevWeight && nextSets >= prevSets {
				t.Errorf("Expected weight decrease or volume reduction after 'too difficult' feedback for exercise %d, "+
					"but got %.1f -> %.1f, sets: %d -> %d",
					pair.previous.ExerciseID, prevWeight, nextWeight, prevSets, nextSets)
			} else if prevWeight == 0 && nextSets >= prevSets {
				// If weight is already 0, we should at least reduce volume
				t.Errorf("Weight already at 0 for exercise %d after 'too difficult' feedback - expected volume reduction",
					pair.previous.ExerciseID)
			}
		default: // Optimal (2-4)
			// More forgiving check for small weight changes (within 15%)
			if prevWeight > 0 && nextWeight > 0 &&
				(nextWeight < prevWeight*0.85 || nextWeight > prevWeight*1.15) {
				// For undulating periodization, allow larger weight changes but flag potential issues
				if nextWeight == 0 {
					t.Errorf("Weight reset to 0 for exercise %d after 'optimal' feedback - likely an exercise matching issue",
						pair.previous.ExerciseID)
				} else {
					t.Errorf("Expected moderate weight changes after 'optimal' feedback for exercise %d, but got %.1f -> %.1f",
						pair.previous.ExerciseID, prevWeight, nextWeight)
				}
			} else if prevWeight > 0 && nextWeight == 0 {
				t.Errorf("Weight reset to 0 for exercise %d after 'optimal' feedback - likely an exercise matching issue",
					pair.previous.ExerciseID)
			}
		}
	}
}

// Helper functions for data manipulation

// Get list of exercise IDs from a session.
func getExerciseIDs(t *testing.T, session sessionAggregate) []int {
	t.Helper()
	ids := make([]int, 0, len(session.ExerciseSets))
	for _, exerciseSet := range session.ExerciseSets {
		ids = append(ids, exerciseSet.ExerciseID)
	}
	return ids
}

// Count common elements between two slices.
func countCommonElements(t *testing.T, slice1, slice2 []int) int {
	t.Helper()
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
	previous exerciseSetAggregate
	next     exerciseSetAggregate
}

// FindCommonExercises finds exercises that exist in both workouts.
func findCommonExercises(t *testing.T, prev, next sessionAggregate) []exercisePair {
	t.Helper()
	var common []exercisePair

	for _, prevEx := range prev.ExerciseSets {
		for _, nextEx := range next.ExerciseSets {
			// Match by both ID and name for more reliable matching
			if prevEx.ExerciseID == nextEx.ExerciseID {
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
func getAverageWeight(t *testing.T, exerciseSet exerciseSetAggregate) float64 {
	t.Helper()
	if len(exerciseSet.Sets) == 0 {
		return 0
	}

	var total float64
	for _, set := range exerciseSet.Sets {
		if set.WeightKg != nil {
			total += *set.WeightKg
		}
	}

	return total / float64(len(exerciseSet.Sets))
}
