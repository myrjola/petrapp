package domain

import (
	"fmt"
	"time"
)

// restPushLeadSeconds is how far before the actual rest end the push fires.
// A heads-up gives the user time to put down their phone and set up before
// the next set is due — the on-screen "Ready" chime in sets-container.gohtml
// still marks the actual end of rest.
const restPushLeadSeconds = 10

// RestPushAction discriminates the three things the scheduler may be asked
// to do for a slot after a state change.
type RestPushAction int

const (
	// RestPushActionNoOp means the slot transitioned but no push should
	// be touched (e.g. the exercise has no defined rest period).
	RestPushActionNoOp RestPushAction = iota
	// RestPushActionSchedule means a push for the next incomplete set
	// should be scheduled at FireAt with Payload.
	RestPushActionSchedule
	// RestPushActionCancel means any pending push for the slot should be
	// removed (the slot has no incomplete sets left, or the exercise has
	// no rest defined; the latter is handled by NoOp, so Cancel implies
	// "all sets done").
	RestPushActionCancel
)

// RestPushPayload carries the user-visible content of a scheduled rest push.
// The scheduler / sender don't inspect these fields; the service layer
// marshals them into JSON the service worker reads.
type RestPushPayload struct {
	Title         string
	Body          string
	ExerciseName  string
	NextSetNumber int
	SetsTotal     int
}

// RestPushDecision is the value PlanRestPush returns. FireAt and Payload are
// only meaningful when Action == RestPushActionSchedule.
type RestPushDecision struct {
	Action  RestPushAction
	FireAt  time.Time
	Payload RestPushPayload
}

// PlanRestPush inspects the slot after a state change and decides what the
// push scheduler should do. completedAt is the moment the mutation happened
// — used as the rest-clock zero point. The rule is uniform across triggers
// (warmup-complete and set-complete) because both ask the same question:
// "what is the first incomplete set in this slot?".
func PlanRestPush(
	slot ExerciseSlot,
	goal SessionGoal,
	isDeload bool,
	completedAt time.Time,
) RestPushDecision {
	nextIdx := -1
	for i := range slot.Sets {
		if slot.Sets[i].CompletedAt == nil {
			nextIdx = i
			break
		}
	}
	if nextIdx == -1 {
		// No incomplete sets remain — every set in this slot is done.
		return RestPushDecision{Action: RestPushActionCancel} //nolint:exhaustruct // FireAt/Payload unused for Cancel.
	}

	restSeconds := RestSecondsFor(slot.Exercise, goal, isDeload)
	if restSeconds <= 0 {
		return RestPushDecision{Action: RestPushActionNoOp} //nolint:exhaustruct // FireAt/Payload unused for NoOp.
	}

	nextSetNumber := nextIdx + 1
	setsTotal := len(slot.Sets)
	return RestPushDecision{
		Action: RestPushActionSchedule,
		FireAt: completedAt.Add(time.Duration(restSeconds-restPushLeadSeconds) * time.Second),
		Payload: RestPushPayload{
			Title:         "Rest over",
			Body:          fmt.Sprintf("Time for set %d of %d — %s", nextSetNumber, setsTotal, slot.Exercise.Name),
			ExerciseName:  slot.Exercise.Name,
			NextSetNumber: nextSetNumber,
			SetsTotal:     setsTotal,
		},
	}
}
