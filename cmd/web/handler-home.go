package main

import (
	"github.com/myrjola/petrapp/internal/workout"
	"net/http"
	"time"
)

const (
	maxDifficultyStars = 5
	percentMultiplier  = 100
)

// isWorkoutScheduled determines if a workout is scheduled for the given date based on user preferences.
func isWorkoutScheduled(date time.Time, preferences workout.Preferences) bool {
	switch date.Weekday() {
	case time.Monday:
		return preferences.Monday
	case time.Tuesday:
		return preferences.Tuesday
	case time.Wednesday:
		return preferences.Wednesday
	case time.Thursday:
		return preferences.Thursday
	case time.Friday:
		return preferences.Friday
	case time.Saturday:
		return preferences.Saturday
	case time.Sunday:
		return preferences.Sunday
	default:
		return false
	}
}

type homeTemplateData struct {
	BaseTemplateData
	// Days contains the workout sessions for the current week
	Days []dayView
}

// DayView represents a single day's view data.
type dayView struct {
	// Date is the date of this day
	Date time.Time
	// Name is the weekday name (e.g. "Monday")
	Name string
	// IsToday indicates if this is the current day
	IsToday bool
	// IsPast indicates if this day is in the past
	IsPast bool
	// IsScheduled indicates if a workout is scheduled for this day
	IsScheduled bool
	// WorkoutStatus indicates the completion status of the workout
	WorkoutStatus string
	// CompletedSets is the number of completed sets
	CompletedSets int
	// TotalSets is the total number of sets in the workout
	TotalSets int
	// ProgressPercent is the completion percentage (0-100)
	ProgressPercent int
	// DifficultyRating is the user's difficulty rating (1-5) if provided
	DifficultyRating *int
	// DifficultyStars represents filled and empty stars for display
	DifficultyStars []bool
}

func toDays(sessions []workout.Session, preferences workout.Preferences) []dayView {
	today := time.Now()
	days := make([]dayView, len(sessions))

	for i, session := range sessions {
		date := session.Date

		// Determine if this day is scheduled based on preferences
		isScheduled := isWorkoutScheduled(date, preferences)

		// Determine workout status based on timestamps and schedule
		var workoutStatus string
		switch {
		case !isScheduled && session.StartedAt.IsZero():
			workoutStatus = "unscheduled"
		case session.StartedAt.IsZero():
			workoutStatus = "not_started"
		case session.CompletedAt.IsZero():
			workoutStatus = "in_progress"
		default:
			workoutStatus = "completed"
		}

		// Count completed and total sets
		completedSets := 0
		totalSets := 0
		for _, exerciseSet := range session.ExerciseSets {
			for _, set := range exerciseSet.Sets {
				totalSets++
				if set.CompletedAt != nil {
					completedSets++
				}
			}
		}

		// Calculate progress percentage
		progressPercent := 0
		if totalSets > 0 {
			progressPercent = (completedSets * percentMultiplier) / totalSets
		}

		// Prepare difficulty stars
		var difficultyStars []bool
		if session.DifficultyRating != nil {
			difficultyStars = make([]bool, maxDifficultyStars)
			for j := range maxDifficultyStars {
				difficultyStars[j] = j < *session.DifficultyRating
			}
		}

		days[i] = dayView{
			Date:             date,
			Name:             date.Format("Monday"),
			IsToday:          date.Format("2006-01-02") == today.Format("2006-01-02"),
			IsPast:           date.Before(today),
			IsScheduled:      isScheduled,
			WorkoutStatus:    workoutStatus,
			CompletedSets:    completedSets,
			TotalSets:        totalSets,
			ProgressPercent:  progressPercent,
			DifficultyRating: session.DifficultyRating,
			DifficultyStars:  difficultyStars,
		}
	}

	return days
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	data := homeTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Days:             nil,
	}

	// Only fetch workout data for authenticated users
	if data.Authenticated {
		sessions, err := app.workoutService.ResolveWeeklySchedule(r.Context())
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		preferences, err := app.workoutService.GetUserPreferences(r.Context())
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		data.Days = toDays(sessions, preferences)
	}

	app.render(w, r, http.StatusOK, "home", data)
}
