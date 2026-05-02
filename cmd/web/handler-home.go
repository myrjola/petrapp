package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/workout"
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
		return preferences.Monday()
	case time.Tuesday:
		return preferences.Tuesday()
	case time.Wednesday:
		return preferences.Wednesday()
	case time.Thursday:
		return preferences.Thursday()
	case time.Friday:
		return preferences.Friday()
	case time.Saturday:
		return preferences.Saturday()
	case time.Sunday:
		return preferences.Sunday()
	default:
		return false
	}
}

type homeTemplateData struct {
	BaseTemplateData
	// Days contains the workout sessions for the current week
	Days []dayView
	// MuscleBalance summarises weekly volume per muscle group, grouped by region.
	// Empty for unauthenticated users; Regions is empty when the week has no exercises.
	MuscleBalance muscleBalanceView
}

// muscleBalanceView wraps the per-region groupings rendered below the weekly schedule.
type muscleBalanceView struct {
	Regions []muscleRegionView
}

// muscleRegionView is one anatomical group (e.g. "Upper Push") and the bars within it.
type muscleRegionView struct {
	Name   string
	Groups []muscleGroupBarView
}

// muscleGroupBarView holds the per-row data needed to render one bar.
// All percent fields are 0..100 relative to a shared scale so bars are visually comparable.
type muscleGroupBarView struct {
	Name           string
	Slug           string
	CompletedLoad  float64
	PlannedLoad    float64
	TargetSets     int
	HasTarget      bool
	FillPercent    int
	PlannedPercent int
	TargetPercent  int
	Status         string
}

// Status values for a muscle group bar; these drive the fill colour in the template.
const (
	muscleStatusUnder    = "under"
	muscleStatusOnTarget = "on-target"
	muscleStatusOver     = "over"
	muscleStatusNoTarget = "no-target"

	// scaleHeadroom adds 10% headroom above the largest planned/target value so an
	// on-target bar doesn't pin the right edge.
	scaleHeadroom = 1.1
	// minScale guarantees a non-zero divisor when the week is empty (all volumes 0).
	minScale = 10.0
	// overTargetMultiplier above this many times the target a bar is flagged as
	// over-prescribed. Picked permissively because going somewhat above target is fine;
	// far above suggests redundant programming.
	overTargetMultiplier = 1.5
)

// regionOrder returns the display order for muscle group regions in the UI.
func regionOrder() []workout.MuscleGroupRegion {
	return []workout.MuscleGroupRegion{
		workout.RegionUpperPush,
		workout.RegionUpperPull,
		workout.RegionLegs,
		workout.RegionCore,
		workout.RegionOther,
	}
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
// Sets completed on a day other than the scheduled date still count toward progress, so session-level
// StartedAt/CompletedAt are not the sole source of truth.
func determineWorkoutStatus(session workout.Session, isScheduled bool, completedSets, totalSets int) string {
	allSetsCompleted := totalSets > 0 && completedSets == totalSets
	hasStarted := !session.StartedAt.IsZero() || completedSets > 0

	switch {
	case !session.CompletedAt.IsZero() || allSetsCompleted:
		return statusCompleted
	case hasStarted:
		return statusInProgress
	case !isScheduled:
		return statusUnscheduled
	default:
		return statusNotStarted
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
		if isToday {
			return &workoutAction{
				StartWorkout: true,
				Label:        "Start Extra Workout",
			}
		}
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
	case statusUpcoming:
		return &workoutAction{
			StartWorkout: true,
			Label:        "Start Early",
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

		completedSets, totalSets, progressPercent := calculateProgress(session)
		workoutStatus := determineWorkoutStatus(session, isScheduled, completedSets, totalSets)
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

// toMuscleBalance turns the workout service's flat volume list into a regional
// view-model with pre-computed bar percentages. All bars share one scale so the
// visualization is meaningful at a glance: the largest of (max planned load, max
// target) sets the right edge, plus 10% headroom. Regions with no bars are omitted.
func toMuscleBalance(volumes []workout.MuscleGroupVolume) muscleBalanceView {
	if len(volumes) == 0 {
		return muscleBalanceView{Regions: nil}
	}

	scale := minScale
	for _, v := range volumes {
		if v.PlannedLoad > scale {
			scale = v.PlannedLoad
		}
		if t := float64(v.TargetSets); t > scale {
			scale = t
		}
	}
	scale *= scaleHeadroom

	byRegion := make(map[workout.MuscleGroupRegion][]muscleGroupBarView)
	for _, v := range volumes {
		region := workout.RegionFor(v.Name)
		byRegion[region] = append(byRegion[region], muscleGroupBarView{
			Name:           v.Name,
			Slug:           muscleGroupSlug(v.Name),
			CompletedLoad:  v.CompletedLoad,
			PlannedLoad:    v.PlannedLoad,
			TargetSets:     v.TargetSets,
			HasTarget:      v.TargetSets > 0,
			FillPercent:    int(v.CompletedLoad / scale * percentMultiplier),
			PlannedPercent: int(v.PlannedLoad / scale * percentMultiplier),
			TargetPercent:  int(float64(v.TargetSets) / scale * percentMultiplier),
			Status:         muscleStatus(v.PlannedLoad, v.TargetSets),
		})
	}

	order := regionOrder()
	regions := make([]muscleRegionView, 0, len(order))
	for _, r := range order {
		groups := byRegion[r]
		if len(groups) == 0 {
			continue
		}
		regions = append(regions, muscleRegionView{
			Name:   string(r),
			Groups: groups,
		})
	}
	return muscleBalanceView{Regions: regions}
}

// muscleGroupSlug renders a muscle group name as a CSS-safe slug
// (e.g. "Upper Back" → "upper-back") for use in attribute selectors.
func muscleGroupSlug(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// muscleStatus classifies a muscle group's planned weekly load against its target.
// Groups without a seeded target are reported as "no-target" so the UI can render
// them informationally without making a value judgment.
func muscleStatus(planned float64, target int) string {
	if target <= 0 {
		return muscleStatusNoTarget
	}
	t := float64(target)
	switch {
	case planned < t:
		return muscleStatusUnder
	case planned <= overTargetMultiplier*t:
		return muscleStatusOnTarget
	default:
		return muscleStatusOver
	}
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	data := homeTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Days:             nil,
		MuscleBalance:    muscleBalanceView{Regions: nil},
	}

	// Only fetch workout data for authenticated users
	if data.Authenticated {
		preferences, err := app.workoutService.GetUserPreferences(r.Context())
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		if preferences.IsEmpty() {
			redirect(w, r, "/schedule")
			return
		}

		sessions, err := app.workoutService.ResolveWeeklySchedule(r.Context())
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		volumes, err := app.workoutService.WeeklyMuscleGroupVolume(r.Context(), sessions)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		data.Days = toDays(sessions, preferences)
		data.MuscleBalance = toMuscleBalance(volumes)
	}

	app.render(w, r, http.StatusOK, "home", data)
}
