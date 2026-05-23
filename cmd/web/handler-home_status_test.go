package main

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestDetermineWorkoutStatus(t *testing.T) {
	now := time.Now()
	newSession := func(startedAt, completedAt time.Time) domain.Session {
		return domain.Session{
			Date:              time.Time{},
			DifficultyRating:  nil,
			StartedAt:         startedAt,
			CompletedAt:       completedAt,
			ExerciseSets:      nil,
			PeriodizationType: "",
			IsDeload:          false,
		}
	}

	tests := []struct {
		name          string
		session       domain.Session
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

func TestCalculateWorkoutAction(t *testing.T) {
	tests := []struct {
		name             string
		status           string
		isToday          bool
		wantNil          bool
		wantStartWorkout bool
		wantIsCTA        bool
		wantLabel        string
	}{
		{
			name:             "today: scheduled, not started",
			status:           statusToday,
			isToday:          true,
			wantNil:          false,
			wantStartWorkout: true,
			wantIsCTA:        true,
			wantLabel:        "Start Workout",
		},
		{
			name:             "in progress: continue without re-starting",
			status:           statusInProgress,
			isToday:          false,
			wantNil:          false,
			wantStartWorkout: false,
			wantIsCTA:        false,
			wantLabel:        "Continue Workout",
		},
		{
			name:             "completed past day: view details",
			status:           statusCompleted,
			isToday:          false,
			wantNil:          false,
			wantStartWorkout: false,
			wantIsCTA:        false,
			wantLabel:        "View Details",
		},
		{
			name:             "missed past day: clicking 'Start Late' must POST start",
			status:           statusPastIncomplete,
			isToday:          false,
			wantNil:          false,
			wantStartWorkout: true,
			wantIsCTA:        false,
			wantLabel:        "Start Late",
		},
		{
			name:             "upcoming day: start early posts start",
			status:           statusUpcoming,
			isToday:          false,
			wantNil:          false,
			wantStartWorkout: true,
			wantIsCTA:        false,
			wantLabel:        "Start Early",
		},
		{
			name:             "unscheduled day, today: start extra workout posts start",
			status:           statusUnscheduled,
			isToday:          true,
			wantNil:          false,
			wantStartWorkout: true,
			wantIsCTA:        false,
			wantLabel:        "Start Extra Workout",
		},
		{
			name:             "unscheduled day, not today: no action",
			status:           statusUnscheduled,
			isToday:          false,
			wantNil:          true,
			wantStartWorkout: false,
			wantIsCTA:        false,
			wantLabel:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateWorkoutAction(tt.status, tt.isToday)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("calculateWorkoutAction() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("calculateWorkoutAction() = nil, want non-nil")
			}
			if got.StartWorkout != tt.wantStartWorkout {
				t.Errorf("StartWorkout = %v, want %v", got.StartWorkout, tt.wantStartWorkout)
			}
			if got.IsCTA != tt.wantIsCTA {
				t.Errorf("IsCTA = %v, want %v", got.IsCTA, tt.wantIsCTA)
			}
			if got.Label != tt.wantLabel {
				t.Errorf("Label = %q, want %q", got.Label, tt.wantLabel)
			}
		})
	}
}

func TestShouldShowRibbon(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status string
		want   bool
	}{
		{statusToday, true},
		{statusInProgress, true},
		{statusCompleted, true},
		{statusPastIncomplete, true},
		{statusUpcoming, false},
		{statusUnscheduled, false},
		{statusNotStarted, false},
	} {
		t.Run(tc.status, func(t *testing.T) {
			t.Parallel()
			if got := shouldShowRibbon(tc.status); got != tc.want {
				t.Errorf("shouldShowRibbon(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}
