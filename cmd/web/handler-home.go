package main

import (
	"github.com/myrjola/petrapp/internal/workout"
	"net/http"
	"time"
)

const (
	maxDifficultyStars = 5
	percentMultiplier  = 100

	// Workout statuses.
	statusUnscheduled = "unscheduled"
	statusNotStarted  = "not_started"
	statusInProgress  = "in_progress"
	statusCompleted   = "completed"

	// Display statuses.
	statusToday          = "today"
	statusUpcoming       = "upcoming"
	statusPastIncomplete = "past-incomplete"
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

// dayView represents a single day's view data.
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
	// Status is the final computed status for display and CSS data attributes
	Status string
	// StatusLabel is the human-readable status label
	StatusLabel string
	// CompletedSets is the number of completed sets
	CompletedSets int
	// TotalSets is the total number of sets in the workout
	TotalSets int
	// ProgressPercent is the completion percentage (0-100)
	ProgressPercent int
	// ShouldShowProgress indicates if progress info should be displayed
	ShouldShowProgress bool
	// DifficultyRating is the user's difficulty rating (1-5) if provided
	DifficultyRating *int
	// DifficultyStars represents filled and empty stars for display
	DifficultyStars []bool
	// Action contains the workout action data for this day
	Action *workoutAction
}

// workoutAction represents an action that can be taken on a workout day.
type workoutAction struct {
	// StartWorkout indicates if we need to start a workout instead of navigating to it
	StartWorkout bool
	// Label is the button/link text
	Label string
}

// determineWorkoutStatus determines the workout status based on session data and schedule.
func determineWorkoutStatus(session workout.Session, isScheduled bool) string {
	switch {
	case !isScheduled && session.StartedAt.IsZero():
		return statusUnscheduled
	case session.StartedAt.IsZero():
		return statusNotStarted
	case session.CompletedAt.IsZero():
		return statusInProgress
	default:
		return statusCompleted
	}
}

// calculateProgress counts completed and total sets and returns the progress data.
func calculateProgress(session workout.Session) (int, int, int) {
	var completedSets, totalSets, progressPercent int

	for _, exerciseSet := range session.ExerciseSets {
		for _, set := range exerciseSet.Sets {
			totalSets++
			if set.CompletedAt != nil {
				completedSets++
			}
		}
	}

	if totalSets > 0 {
		progressPercent = (completedSets * percentMultiplier) / totalSets
	}

	return completedSets, totalSets, progressPercent
}

// prepareDifficultyStars creates the difficulty stars array for display.
func prepareDifficultyStars(rating *int) []bool {
	if rating == nil {
		return nil
	}

	stars := make([]bool, maxDifficultyStars)
	for j := range maxDifficultyStars {
		stars[j] = j < *rating
	}
	return stars
}

// calculateDisplayStatus determines the final status and label for display.
func calculateDisplayStatus(workoutStatus string, isToday, isPast bool) (string, string) {
	switch {
	case workoutStatus == statusCompleted:
		return statusCompleted, "Completed"
	case workoutStatus == statusInProgress:
		return statusInProgress, "In Progress"
	case workoutStatus == statusUnscheduled:
		return statusUnscheduled, "Not Scheduled"
	case workoutStatus == statusNotStarted && isToday:
		return statusToday, "Today"
	case workoutStatus == statusNotStarted && isPast:
		return statusPastIncomplete, "Missed"
	case workoutStatus == statusNotStarted:
		return statusUpcoming, "Upcoming"
	default:
		return statusUpcoming, "Upcoming"
	}
}

// calculateWorkoutAction determines the action for the workout based on status and timing.
func calculateWorkoutAction(status string, isToday bool) *workoutAction {
	switch status {
	case statusUnscheduled:
		return nil
	case statusToday:
		return &workoutAction{
			StartWorkout: true,
			Label:        "Start Workout",
		}
	case statusInProgress:
		if isToday {
			return &workoutAction{
				StartWorkout: false,
				Label:        "Continue Workout",
			}
		}
		return &workoutAction{
			StartWorkout: false,
			Label:        "Complete Workout",
		}
	case statusCompleted:
		if isToday {
			return &workoutAction{
				StartWorkout: false,
				Label:        "Review Workout",
			}
		}
		return &workoutAction{
			StartWorkout: false,
			Label:        "View Details",
		}
	case statusPastIncomplete:
		return &workoutAction{
			StartWorkout: false,
			Label:        "Start Late",
		}
	default:
		return nil
	}
}

func toDays(sessions []workout.Session, preferences workout.Preferences) []dayView {
	today := time.Now()
	days := make([]dayView, len(sessions))

	for i, session := range sessions {
		date := session.Date
		isScheduled := isWorkoutScheduled(date, preferences)
		isToday := date.Format("2006-01-02") == today.Format("2006-01-02")
		isPast := date.Before(today)

		workoutStatus := determineWorkoutStatus(session, isScheduled)
		completedSets, totalSets, progressPercent := calculateProgress(session)
		difficultyStars := prepareDifficultyStars(session.DifficultyRating)
		status, statusLabel := calculateDisplayStatus(workoutStatus, isToday, isPast)
		action := calculateWorkoutAction(status, isToday)

		days[i] = dayView{
			Date:               date,
			Name:               date.Format("Monday"),
			IsToday:            isToday,
			IsPast:             isPast,
			IsScheduled:        isScheduled,
			Status:             status,
			StatusLabel:        statusLabel,
			CompletedSets:      completedSets,
			TotalSets:          totalSets,
			ProgressPercent:    progressPercent,
			ShouldShowProgress: totalSets > 0 && status != statusUnscheduled,
			DifficultyRating:   session.DifficultyRating,
			DifficultyStars:    difficultyStars,
			Action:             action,
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
