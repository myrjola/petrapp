package main

import (
	"github.com/myrjola/petrapp/internal/workout"
	"net/http"
	"time"
)

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
	// Status represents the workout status (e.g. "Done", "Skipped", "Rest day", "Planned")
	Status string
	// IsToday indicates if this is the current day
	IsToday bool
	// IsPast indicates if this day is in the past
	IsPast bool
	// HasWorkout indicates if a workout is scheduled for this day
	HasWorkout bool
}

func toDays(sessions []workout.Session) []dayView {
	today := time.Now()
	days := make([]dayView, len(sessions))

	for i, session := range sessions {
		date := session.WorkoutDate
		days[i] = dayView{
			Date:       date,
			Name:       date.Format("Monday"),
			Status:     string(session.Status),
			IsToday:    date.Format("2006-01-02") == today.Format("2006-01-02"),
			IsPast:     date.Before(today),
			HasWorkout: session.Status != workout.StatusRest,
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

		data.Days = toDays(sessions)
	}

	app.render(w, r, http.StatusOK, "home", data)
}
