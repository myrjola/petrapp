package main

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/workout"
)

func TestDetermineWorkoutStatus(t *testing.T) {
	now := time.Now()
	newSession := func(startedAt, completedAt time.Time) workout.Session {
		return workout.Session{
			Date:              time.Time{},
			DifficultyRating:  nil,
			StartedAt:         startedAt,
			CompletedAt:       completedAt,
			ExerciseSets:      nil,
			PeriodizationType: "",
		}
	}

	tests := []struct {
		name          string
		session       workout.Session
		isScheduled   bool
		completedSets int
		totalSets     int
		want          string
	}{
		{
			name:          "scheduled day with no activity",
			session:       newSession(time.Time{}, time.Time{}),
			isScheduled:   true,
			completedSets: 0,
			totalSets:     9,
			want:          statusNotStarted,
		},
		{
			name:          "unscheduled day with no activity",
			session:       newSession(time.Time{}, time.Time{}),
			isScheduled:   false,
			completedSets: 0,
			totalSets:     0,
			want:          statusUnscheduled,
		},
		{
			name:          "session started but no sets done",
			session:       newSession(now, time.Time{}),
			isScheduled:   true,
			completedSets: 0,
			totalSets:     9,
			want:          statusInProgress,
		},
		{
			name:          "some sets completed without StartedAt set",
			session:       newSession(time.Time{}, time.Time{}),
			isScheduled:   true,
			completedSets: 3,
			totalSets:     9,
			want:          statusInProgress,
		},
		{
			name:          "all sets completed without CompletedAt set",
			session:       newSession(time.Time{}, time.Time{}),
			isScheduled:   true,
			completedSets: 9,
			totalSets:     9,
			want:          statusCompleted,
		},
		{
			name:          "session formally completed",
			session:       newSession(now, now),
			isScheduled:   true,
			completedSets: 9,
			totalSets:     9,
			want:          statusCompleted,
		},
		{
			name:          "session completed flag set even without any sets logged",
			session:       newSession(time.Time{}, now),
			isScheduled:   true,
			completedSets: 0,
			totalSets:     9,
			want:          statusCompleted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineWorkoutStatus(tt.session, tt.isScheduled, tt.completedSets, tt.totalSets)
			if got != tt.want {
				t.Errorf("determineWorkoutStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
